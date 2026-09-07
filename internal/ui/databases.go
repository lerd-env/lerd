package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dbview"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/serviceops"
)

// dbEngineResponse is one database engine with the databases it holds.
// Capabilities are read off the engine's declared entity actions, so an engine
// whose preset declares no create or export shows its databases without
// offering operations it can't perform.
type dbEngineResponse struct {
	Service          string            `json:"service"`
	Family           string            `json:"family"`
	Status           string            `json:"status"`
	Port             int               `json:"port,omitempty"`
	Icon             string            `json:"icon,omitempty"`
	ConnectionURL    string            `json:"connection_url,omitempty"`
	SupportsCreate   bool              `json:"supports_create"`
	SupportsDrop     bool              `json:"supports_drop"`
	SupportsExport   bool              `json:"supports_export"`
	SupportsImport   bool              `json:"supports_import"`
	SupportsSnapshot bool              `json:"supports_snapshot"`
	DumpFormat       string            `json:"dump_format,omitempty"`
	Databases        []dbEntryResponse `json:"databases"`
	Error            string            `json:"error,omitempty"`
}

// dbEntryResponse is a single database and the snapshots taken of it. Site is
// the domain of the linked site that owns the database, when one does, and
// Branch names the worktree when the database is that branch's isolated one.
type dbEntryResponse struct {
	Name      string             `json:"name"`
	SizeBytes int64              `json:"size_bytes"`
	Site      string             `json:"site,omitempty"`
	Branch    string             `json:"branch,omitempty"`
	Snapshots []snapshotResponse `json:"snapshots"`
}

// snapshotResponse is a stored snapshot plus what retention has in store for it,
// computed from the live policy rather than stored, so a schedule the user just
// changed is reflected the next time the list is drawn.
type snapshotResponse struct {
	serviceops.Snapshot
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	RunsLeft  int        `json:"runs_left,omitempty"`
	Estimated bool       `json:"estimated,omitempty"`
}

// withRetention pairs each snapshot with its expiry under the current policy.
func withRetention(snaps []serviceops.Snapshot) []snapshotResponse {
	cfg, _ := config.LoadGlobal()
	expiries := serviceops.SnapshotExpiries(serviceops.RetentionPolicy{
		Keep:    cfg.AutoSnapshotKeep(),
		KeepFor: cfg.AutoSnapshotKeepFor(),
		Every:   cfg.AutoSnapshotEvery(),
	}, snaps)

	out := make([]snapshotResponse, 0, len(snaps))
	for i, snap := range snaps {
		row := snapshotResponse{Snapshot: snap, RunsLeft: expiries[i].RunsLeft, Estimated: expiries[i].Estimated}
		if !expiries[i].At.IsZero() {
			at := expiries[i].At
			row.ExpiresAt = &at
		}
		out = append(out, row)
	}
	return out
}

// testingDBSuffix names the paired testing database lerd creates alongside every
// project database.
const testingDBSuffix = dbview.TestingSuffix

// databaseSiteIndexes maps each engine to the databases owned in it. Both this
// surface and the TUI's Databases pane read the same index from dbview.
func databaseSiteIndexes() map[string]map[string]dbview.Owner { return dbview.SiteIndexes() }

// isDatabaseEngine reports whether a service belongs on the Databases surface.
func isDatabaseEngine(name string) bool { return dbview.IsEngine(name) }

// installedDBEngines returns the installed database-engine service names.
func installedDBEngines() []string { return dbview.InstalledEngines() }

// databaseEngine builds one engine's response, introspecting its databases and
// snapshots only when the container is running.
func databaseEngine(name string, siteIndex map[string]dbview.Owner) dbEngineResponse {
	base := buildServiceResponse(name)
	view := dbview.Load(name, siteIndex)
	eng := dbEngineResponse{
		Service:          name,
		Family:           view.Family,
		Status:           base.Status,
		Port:             base.Port,
		Icon:             base.Icon,
		ConnectionURL:    base.ConnectionURL,
		SupportsCreate:   serviceops.DatabaseActionDeclared(name, "create"),
		SupportsDrop:     serviceops.DatabaseActionDeclared(name, "drop"),
		SupportsExport:   serviceops.DatabaseActionDeclared(name, "export"),
		SupportsImport:   serviceops.DatabaseActionDeclared(name, "import"),
		SupportsSnapshot: view.SupportsSnapshot,
		DumpFormat:       serviceops.DatabaseDumpFormat(name),
		Databases:        []dbEntryResponse{},
		Error:            view.Error,
	}
	for _, db := range view.Databases {
		entry := dbEntryResponse{
			Name:      db.Name,
			SizeBytes: db.SizeBytes,
			Site:      db.Owner.Domain,
			Branch:    db.Owner.Branch,
			// A database with no snapshots must serialize as a list, not null.
			Snapshots: []snapshotResponse{},
		}
		if len(db.Snapshots) > 0 {
			entry.Snapshots = withRetention(db.Snapshots)
		}
		eng.Databases = append(eng.Databases, entry)
	}
	return eng
}

// handleDatabases lists every installed database engine and its databases.
func handleDatabases(w http.ResponseWriter, _ *http.Request) {
	names := installedDBEngines()
	indexes := databaseSiteIndexes()
	engines := make([]dbEngineResponse, 0, len(names))
	for _, name := range names {
		engines = append(engines, databaseEngine(name, indexes[name]))
	}
	writeJSON(w, engines)
}

// handleDatabaseAction routes the mutating and export/import endpoints under
// /api/databases/<service>/<action>[/<sub>].
func handleDatabaseAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/databases/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	service := parts[0]
	if !isDatabaseEngine(service) || !serviceops.ServiceInstalled(service) {
		http.Error(w, "unknown database engine", http.StatusNotFound)
		return
	}

	// GET /api/databases/<service> returns just that engine, for its detail tab.
	if len(parts) == 1 {
		if r.Method == http.MethodGet {
			writeJSON(w, databaseEngine(service, databaseSiteIndexes()[service]))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	action := parts[1]

	// Exports stream from the browser and read from disk, so they run without the
	// engine; every other action mutates and requires it running.
	if action == "export" && r.Method == http.MethodGet {
		handleDatabaseExport(w, r, service)
		return
	}
	if action == "snapshot" && len(parts) == 3 && parts[2] == "export" && r.Method == http.MethodGet {
		handleSnapshotExport(w, r, service)
		return
	}
	// Keeping a snapshot only rewrites its sidecar on disk, so it works whether
	// or not the engine is up.
	if action == "snapshot" && len(parts) == 3 && parts[2] == "keep" && r.Method == http.MethodPost {
		handleSnapshotKeep(w, r, service)
		return
	}
	if status, _ := podman.UnitStatus("lerd-" + service); status != "active" {
		writeDBError(w, "start the engine before running database operations")
		return
	}

	switch {
	case action == "create":
		handleDatabaseCreate(w, r, service)
	case action == "drop":
		handleDatabaseDrop(w, r, service)
	case action == "import":
		handleDatabaseImport(w, r, service)
	case action == "snapshot" && len(parts) == 2:
		handleSnapshotCreate(w, r, service)
	case action == "snapshot" && len(parts) == 3 && parts[2] == "restore":
		handleSnapshotRestore(w, r, service)
	case action == "snapshot" && len(parts) == 3 && parts[2] == "delete":
		handleSnapshotDelete(w, r, service)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// dbActionResponse is the shared {ok,error} envelope the databases store reads.
// A load that the engine only half swallowed still comes back ok, carrying what
// it complained about, since psql exits clean either way.
type dbActionResponse struct {
	OK      bool                     `json:"ok"`
	Error   string                   `json:"error,omitempty"`
	Errors  int                      `json:"errors,omitempty"`
	Issues  []serviceops.ImportIssue `json:"issues,omitempty"`
	Omitted int                      `json:"omitted,omitempty"`
	Skipped []serviceops.ImportIssue `json:"skipped,omitempty"`
	Created []serviceops.ImportIssue `json:"created,omitempty"`
}

func writeDBOK(w http.ResponseWriter) { writeJSON(w, dbActionResponse{OK: true}) }

func writeDBReport(w http.ResponseWriter, rep serviceops.ImportReport) {
	writeJSON(w, dbActionResponse{OK: true, Errors: rep.Errors, Issues: rep.Issues, Omitted: rep.Omitted, Skipped: rep.Skipped, Created: rep.Created})
}
func writeDBError(w http.ResponseWriter, m string) {
	writeJSON(w, dbActionResponse{OK: false, Error: m})
}

// decodeDBBody reads the common {database,name} body used by the mutating
// endpoints; not every field is required by every caller.
func decodeDBBody(r *http.Request) (database, name string, ok bool) {
	var body struct {
		Database string `json:"database"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", "", false
	}
	return strings.TrimSpace(body.Database), strings.TrimSpace(body.Name), true
}

// decodeDropBody reads the drop body, which carries the choice to take the
// paired testing database along with the database named.
func decodeDropBody(r *http.Request) (name string, withTesting, ok bool) {
	var body struct {
		Name    string `json:"name"`
		Testing bool   `json:"testing"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", false, false
	}
	return strings.TrimSpace(body.Name), body.Testing, true
}

// dropTargets is what one drop request removes: the database named, plus the
// testing database it is paired with when the request asks for it. A database
// that is itself the testing half has no pair of its own.
func dropTargets(name string, withTesting bool) []string {
	if !withTesting || strings.HasSuffix(name, testingDBSuffix) {
		return []string{name}
	}
	return []string{name, name + testingDBSuffix}
}

// requireDatabaseName rejects a database name that could escape its snapshot
// path or its SQL quoting, so nothing unvalidated reaches serviceops.
func requireDatabaseName(w http.ResponseWriter, database string) bool {
	if err := serviceops.ValidateDatabaseName(database); err != nil {
		writeDBError(w, err.Error())
		return false
	}
	return true
}

func handleDatabaseCreate(w http.ResponseWriter, r *http.Request, service string) {
	_, name, ok := decodeDBBody(r)
	if !ok || !requireDatabaseName(w, name) {
		return
	}
	// CreateDatabase quietly no-ops for an engine without a declared create, so
	// the gate here is what keeps "not supported" from reading as "exists".
	if !serviceops.DatabaseActionDeclared(service, "create") {
		writeDBError(w, fmt.Sprintf("%s does not support creating databases", service))
		return
	}
	created, err := serviceops.CreateDatabase(service, name)
	if err != nil {
		writeDBError(w, err.Error())
		return
	}
	if !created {
		writeDBError(w, fmt.Sprintf("database %q already exists", name))
		return
	}
	writeDBOK(w)
}

func handleDatabaseDrop(w http.ResponseWriter, r *http.Request, service string) {
	name, withTesting, ok := decodeDropBody(r)
	if !ok {
		return
	}
	// Both halves are validated before either is touched, so a sibling the
	// suffix pushes past the length limit never costs the database it tests.
	targets := dropTargets(name, withTesting)
	for _, target := range targets {
		if !requireDatabaseName(w, target) {
			return
		}
	}
	if !serviceops.DatabaseActionDeclared(service, "drop") {
		writeDBError(w, fmt.Sprintf("%s does not support dropping databases", service))
		return
	}
	for _, target := range targets {
		if _, err := serviceops.DropDatabase(service, target); err != nil {
			writeDBError(w, fmt.Sprintf("dropping %s: %v", target, err))
			return
		}
	}
	writeDBOK(w)
}

func handleSnapshotCreate(w http.ResponseWriter, r *http.Request, service string) {
	database, name, ok := decodeDBBody(r)
	if !ok || !requireDatabaseName(w, database) {
		return
	}
	target := serviceops.SnapshotTarget{Service: service, Family: config.FamilyOfName(service), Database: database}
	meta := serviceops.SnapshotMeta{Site: snapshotSiteName(service, database)}
	if _, err := serviceops.CreateSnapshot(target, name, meta, nil); err != nil {
		writeDBError(w, err.Error())
		return
	}
	writeDBOK(w)
}

// snapshotSiteName resolves the site a database belongs to, by name rather than
// by domain: the scheduler and the CLI both record the name, and a column that
// mixed the two would read as two different sites for one project.
func snapshotSiteName(service, database string) string {
	owner := databaseSiteIndexes()[service][database]
	if owner.Domain == "" {
		return ""
	}
	site, err := config.FindSiteByDomain(owner.Domain)
	if err != nil {
		return ""
	}
	return site.Name
}

func handleSnapshotRestore(w http.ResponseWriter, r *http.Request, service string) {
	database, name, ok := decodeDBBody(r)
	if !ok || name == "" {
		writeDBError(w, "a database and snapshot name are required")
		return
	}
	if !requireDatabaseName(w, database) {
		return
	}
	target := serviceops.SnapshotTarget{Service: service, Family: config.FamilyOfName(service), Database: database}
	rep, err := serviceops.RestoreSnapshot(target, name, nil)
	if err != nil {
		writeDBError(w, err.Error())
		return
	}
	writeDBReport(w, rep)
}

func handleSnapshotDelete(w http.ResponseWriter, r *http.Request, service string) {
	database, name, ok := decodeDBBody(r)
	if !ok || name == "" {
		writeDBError(w, "a database and snapshot name are required")
		return
	}
	if !requireDatabaseName(w, database) {
		return
	}
	if err := serviceops.DeleteSnapshot(service, database, name, false); err != nil {
		writeDBError(w, err.Error())
		return
	}
	writeDBOK(w)
}

// handleSnapshotKeep pins an automatic snapshot so retention leaves it alone, or
// releases it back under retention.
func handleSnapshotKeep(w http.ResponseWriter, r *http.Request, service string) {
	var body struct {
		Database string `json:"database"`
		Name     string `json:"name"`
		Kept     bool   `json:"kept"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeDBError(w, "a database and snapshot name are required")
		return
	}
	if !requireDatabaseName(w, body.Database) {
		return
	}
	if err := serviceops.SetSnapshotKept(service, body.Database, body.Name, false, body.Kept); err != nil {
		writeDBError(w, err.Error())
		return
	}
	writeDBOK(w)
}

// handleDatabaseExport streams a plain SQL dump of ?database=<name> as a
// downloadable file.
func handleDatabaseExport(w http.ResponseWriter, r *http.Request, service string) {
	database := strings.TrimSpace(r.URL.Query().Get("database"))
	if err := serviceops.ValidateDatabaseName(database); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if status, _ := podman.UnitStatus("lerd-" + service); status != "active" {
		http.Error(w, "start the engine before exporting", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", serviceops.ExportFilename(service, database)))
	if err := serviceops.ExportDatabase(service, database, w); err != nil {
		// Headers are already sent, so the browser sees a truncated file; log the
		// cause for the terminal rather than trying to rewrite the response.
		fmt.Printf("database export failed for %s/%s: %v\n", service, database, err)
	}
}

// handleSnapshotExport streams a stored snapshot as a downloadable dump, named
// in the engine's own dump format so a mongo snapshot saves as a .archive
// rather than being mislabelled .sql.
func handleSnapshotExport(w http.ResponseWriter, r *http.Request, service string) {
	database := strings.TrimSpace(r.URL.Query().Get("database"))
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		http.Error(w, "a database and snapshot name are required", http.StatusBadRequest)
		return
	}
	if err := serviceops.ValidateDatabaseName(database); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", serviceops.SnapshotExportFilename(service, name)))
	if err := serviceops.ExportSnapshot(service, database, name, w); err != nil {
		fmt.Printf("snapshot export failed for %s/%s/%s: %v\n", service, database, name, err)
	}
}

// handleDatabaseImport loads an uploaded SQL dump into the database named by the
// form. The parts are walked by hand rather than through ParseMultipartForm,
// which would read the whole request first and spill a large dump to a temp
// file; here the body is read only as fast as the engine swallows it, which is
// what makes the browser's upload progress the progress of the load itself.
func handleDatabaseImport(w http.ResponseWriter, r *http.Request, service string) {
	parts, err := r.MultipartReader()
	if err != nil {
		writeDBError(w, "a dump file is required")
		return
	}
	database := ""
	var opt serviceops.ImportOptions
	for {
		part, err := parts.NextPart()
		if err != nil {
			writeDBError(w, "a dump file is required")
			return
		}
		// The field order is the client's, so the file part is the terminator and
		// everything before it is read as a setting.
		if name := part.FormName(); name != "file" && name != "" {
			val, err := io.ReadAll(io.LimitReader(part, 1024))
			part.Close()
			if err != nil {
				writeDBError(w, "could not read the "+name+" field")
				return
			}
			switch name {
			case "database":
				database = strings.TrimSpace(string(val))
			case "fresh":
				opt.Fresh = strings.TrimSpace(string(val)) == "true"
			}
			continue
		}
		if !requireDatabaseName(w, database) {
			part.Close()
			return
		}
		rep, err := serviceops.ImportDatabase(service, database, part, opt)
		// Whatever the engine left unread is drained so the client still gets the
		// response instead of a reset connection.
		_, _ = io.Copy(io.Discard, part)
		part.Close()
		if err != nil {
			writeDBError(w, err.Error())
			return
		}
		writeDBReport(w, rep)
		return
	}
}
