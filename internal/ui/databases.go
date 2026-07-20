package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/siteinfo"
)

// dbEngineResponse is one database engine with the databases it holds. The tab
// groups engines by family; SQL-only capabilities (create/drop/export/import/
// snapshot) are gated on SupportsCreate / SupportsSnapshot so a document engine
// like mongo shows its databases without offering operations it can't perform.
type dbEngineResponse struct {
	Service          string            `json:"service"`
	Family           string            `json:"family"`
	Status           string            `json:"status"`
	Port             int               `json:"port,omitempty"`
	Icon             string            `json:"icon,omitempty"`
	ConnectionURL    string            `json:"connection_url,omitempty"`
	SupportsCreate   bool              `json:"supports_create"`
	SupportsSnapshot bool              `json:"supports_snapshot"`
	Databases        []dbEntryResponse `json:"databases"`
	Error            string            `json:"error,omitempty"`
}

// dbEntryResponse is a single database and the snapshots taken of it. Site is
// the domain of the linked site that owns the database, when one does.
type dbEntryResponse struct {
	Name      string                `json:"name"`
	SizeBytes int64                 `json:"size_bytes"`
	Site      string                `json:"site,omitempty"`
	Snapshots []serviceops.Snapshot `json:"snapshots"`
}

// databaseSiteIndex maps each database name in the given engine to the domain of
// the site that owns it, read from sites' .env DB_DATABASE. A "<db>_testing"
// database maps to the same site as "<db>", so both link to the same place.
// When a group shares one database across a main site and its secondaries, the
// database belongs to the group main, so a secondary that merely shares it never
// wins over the main.
func databaseSiteIndex(service string) map[string]string {
	reg, err := config.LoadSites()
	if err != nil {
		return nil
	}
	idx := map[string]string{}
	// authoritative[db] is true once db is claimed by a site that owns it rather
	// than a secondary sharing the group's database.
	authoritative := map[string]bool{}
	claim := func(db, domain string, owns bool) {
		if _, seen := idx[db]; !seen || (!authoritative[db] && owns) {
			idx[db] = domain
			authoritative[db] = owns
		}
	}
	for _, s := range reg.Sites {
		if s.Ignored {
			continue
		}
		envPath := filepath.Join(s.Path, ".env")
		host := strings.TrimSpace(envfile.ReadKey(envPath, "DB_HOST"))
		if strings.TrimPrefix(host, "lerd-") != service {
			continue
		}
		db := strings.TrimSpace(envfile.ReadKey(envPath, "DB_DATABASE"))
		if db == "" {
			continue
		}
		owns := !(s.IsGroupSecondary() && s.GroupSharedDB)
		domain := s.PrimaryDomain()
		claim(db, domain, owns)
		claim(db+"_testing", domain, owns)
	}
	return idx
}

// installedDBEngines returns the installed database-engine service names, both
// default-stack (mysql, postgres) and add-on (mariadb, mongo, postgres-pgvector).
// sqlite is a file-based engine with no container, so it is excluded.
func installedDBEngines() []string {
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		if name == "sqlite" || seen[name] || !config.IsDBServiceName(name) {
			return
		}
		if !serviceops.ServiceInstalled(name) {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, name := range siteinfo.KnownServices() {
		add(name)
	}
	if customs, err := config.ListCustomServices(); err == nil {
		for _, svc := range customs {
			add(svc.Name)
		}
	}
	sort.Strings(names)
	return names
}

// databaseEngine builds one engine's response, introspecting its databases and
// snapshots only when the container is running.
func databaseEngine(name string) dbEngineResponse {
	base := buildServiceResponse(name)
	family := config.FamilyOfName(name)
	sqlOps := serviceops.SnapshotFamilySupported(family)
	eng := dbEngineResponse{
		Service:          name,
		Family:           family,
		Status:           base.Status,
		Port:             base.Port,
		Icon:             base.Icon,
		ConnectionURL:    base.ConnectionURL,
		SupportsCreate:   sqlOps,
		SupportsSnapshot: sqlOps,
		Databases:        []dbEntryResponse{},
	}
	if base.Status != "active" {
		return eng
	}
	command := serviceops.IntrospectCommand(name)
	if command == "" {
		return eng
	}
	dbs, err := serviceops.ListDatabases(name, command)
	if err != nil {
		eng.Error = err.Error()
		return eng
	}
	siteIndex := databaseSiteIndex(name)
	for _, db := range dbs {
		entry := dbEntryResponse{
			Name:      db.Name,
			SizeBytes: db.SizeBytes,
			Site:      siteIndex[db.Name],
			Snapshots: []serviceops.Snapshot{},
		}
		if sqlOps {
			if snaps, sErr := serviceops.ListSnapshots(name, db.Name, false); sErr == nil {
				entry.Snapshots = snaps
			}
		}
		eng.Databases = append(eng.Databases, entry)
	}
	return eng
}

// handleDatabases lists every installed database engine and its databases.
func handleDatabases(w http.ResponseWriter, _ *http.Request) {
	names := installedDBEngines()
	engines := make([]dbEngineResponse, 0, len(names))
	for _, name := range names {
		engines = append(engines, databaseEngine(name))
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
	if !config.IsDBServiceName(service) || !serviceops.ServiceInstalled(service) {
		http.Error(w, "unknown database engine", http.StatusNotFound)
		return
	}

	// GET /api/databases/<service> returns just that engine, for its detail tab.
	if len(parts) == 1 {
		if r.Method == http.MethodGet {
			writeJSON(w, databaseEngine(service))
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
type dbActionResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func writeDBOK(w http.ResponseWriter) { writeJSON(w, dbActionResponse{OK: true}) }
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
	_, name, ok := decodeDBBody(r)
	if !ok || !requireDatabaseName(w, name) {
		return
	}
	if _, err := serviceops.DropDatabase(service, name); err != nil {
		writeDBError(w, err.Error())
		return
	}
	writeDBOK(w)
}

func handleSnapshotCreate(w http.ResponseWriter, r *http.Request, service string) {
	database, name, ok := decodeDBBody(r)
	if !ok || !requireDatabaseName(w, database) {
		return
	}
	target := serviceops.SnapshotTarget{Service: service, Family: config.FamilyOfName(service), Database: database}
	if _, err := serviceops.CreateSnapshot(target, name, serviceops.SnapshotMeta{}, nil); err != nil {
		writeDBError(w, err.Error())
		return
	}
	writeDBOK(w)
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
	if err := serviceops.RestoreSnapshot(target, name, nil); err != nil {
		writeDBError(w, err.Error())
		return
	}
	writeDBOK(w)
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
	w.Header().Set("Content-Type", "application/sql")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", database+".sql"))
	if err := serviceops.ExportDatabase(service, database, w); err != nil {
		// Headers are already sent, so the browser sees a truncated file; log the
		// cause for the terminal rather than trying to rewrite the response.
		fmt.Printf("database export failed for %s/%s: %v\n", service, database, err)
	}
}

// handleSnapshotExport streams a stored snapshot as a downloadable .sql dump.
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
	w.Header().Set("Content-Type", "application/sql")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name+".sql"))
	if err := serviceops.ExportSnapshot(service, database, name, w); err != nil {
		fmt.Printf("snapshot export failed for %s/%s/%s: %v\n", service, database, name, err)
	}
}

// handleDatabaseImport loads an uploaded SQL dump into ?database=<name>. The
// upload is streamed into the engine, so a large dump never buffers on disk.
func handleDatabaseImport(w http.ResponseWriter, r *http.Request, service string) {
	database := strings.TrimSpace(r.FormValue("database"))
	if !requireDatabaseName(w, database) {
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeDBError(w, "a dump file is required")
		return
	}
	defer file.Close()
	if err := serviceops.ImportDatabase(service, database, file); err != nil {
		writeDBError(w, err.Error())
		return
	}
	writeDBOK(w)
}
