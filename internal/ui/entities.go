package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/serviceops"
)

// entityKindResponse is one declared entity kind with its live rows, rendered
// generically from the preset declaration: the UI knows only names, columns and
// actions, never the engine.
type entityKindResponse struct {
	Kind    string                 `json:"kind"`
	Label   string                 `json:"label,omitempty"`
	Columns []entityColumnResponse `json:"columns"`
	Actions []entityActionResponse `json:"actions"`
	Rows    []entityRowResponse    `json:"rows"`
	Error   string                 `json:"error,omitempty"`
}

type entityColumnResponse struct {
	Key    string `json:"key"`
	Label  string `json:"label,omitempty"`
	Format string `json:"format,omitempty"`
}

// entityActionResponse names one runnable action. The service-wide export_all
// and import_all variants back snapshots and never render as row actions.
type entityActionResponse struct {
	Name        string `json:"name"`
	Destructive bool   `json:"destructive,omitempty"`
}

// entityRowResponse is one listed entity with the domain of the site that owns
// it, when the kind declares an owner key and a site claims the name.
type entityRowResponse struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
	Site   string   `json:"site,omitempty"`
}

// entityOwnerIndex maps entity names to the domain of the site whose .env
// claims them through the kind's declared owner key. Only sites whose .env
// references this service count, so a project pointed at real AWS never claims
// a local bucket of the same name.
func entityOwnerIndex(service, ownerEnv string) map[string]string {
	if ownerEnv == "" {
		return nil
	}
	reg, err := config.LoadSites()
	if err != nil {
		return nil
	}
	idx := map[string]string{}
	for _, s := range reg.Sites {
		if s.Ignored {
			continue
		}
		vals := envfile.ReadValues(filepath.Join(s.Path, ".env"))
		name := strings.TrimSpace(vals[ownerEnv])
		if name == "" {
			continue
		}
		refsService := false
		for _, v := range vals {
			if strings.Contains(v, "lerd-"+service) {
				refsService = true
				break
			}
		}
		if refsService {
			idx[name] = s.PrimaryDomain()
		}
	}
	return idx
}

// handleEntities serves the generic entity overview:
//
//	GET  /api/entities/<service>                        declared kinds with live rows
//	GET  /api/entities/<service>/<kind>/export?name=    stream one entity's archive
//	POST /api/entities/<service>/<kind>/import          load an uploaded archive
//	POST /api/entities/<service>/<kind>/<action>        {"name": "..."}
func handleEntities(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/entities/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	service := parts[0]
	specs := serviceops.ServiceEntities(service)
	if len(specs) == 0 || !serviceops.ServiceInstalled(service) {
		http.Error(w, "service declares no entities", http.StatusNotFound)
		return
	}

	if len(parts) == 1 && r.Method == http.MethodGet {
		writeJSON(w, entityOverview(service, specs))
		return
	}

	if len(parts) == 3 {
		kind, action := parts[1], parts[2]
		switch {
		case action == "export" && r.Method == http.MethodGet:
			handleEntityExport(w, r, service, kind)
			return
		case action == "import" && r.Method == http.MethodPost:
			handleEntityImport(w, r, service, kind)
			return
		case r.Method == http.MethodPost:
			handleEntityAction(w, r, service, kind, action)
			return
		}
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func entityOverview(service string, specs []config.EntitySpec) []entityKindResponse {
	active := false
	if status, _ := podman.UnitStatus("lerd-" + service); status == "active" {
		active = true
	}
	out := make([]entityKindResponse, 0, len(specs))
	for i := range specs {
		spec := &specs[i]
		kind := entityKindResponse{
			Kind:    spec.Kind,
			Label:   spec.Label,
			Columns: []entityColumnResponse{},
			Actions: []entityActionResponse{},
			Rows:    []entityRowResponse{},
		}
		for _, c := range spec.Columns {
			kind.Columns = append(kind.Columns, entityColumnResponse{Key: c.Key, Label: c.Label, Format: c.Format})
		}
		for name, act := range spec.Actions {
			if name == "export_all" || name == "import_all" {
				continue
			}
			kind.Actions = append(kind.Actions, entityActionResponse{Name: name, Destructive: act.Destructive})
		}
		sortEntityActions(kind.Actions)
		if active {
			rows, err := serviceops.ListEntities(service, spec)
			if err != nil {
				kind.Error = err.Error()
			} else {
				owners := entityOwnerIndex(service, spec.OwnerEnv)
				for _, row := range rows {
					values := row.Values
					if values == nil {
						values = []string{}
					}
					kind.Rows = append(kind.Rows, entityRowResponse{Name: row.Name, Values: values, Site: owners[row.Name]})
				}
			}
		}
		out = append(out, kind)
	}
	return out
}

// sortEntityActions imposes a sensible order on the declared map: create
// first, streaming next, destructive last, alphabetical within a group.
func sortEntityActions(actions []entityActionResponse) {
	rank := func(a entityActionResponse) string {
		switch {
		case a.Name == "create":
			return "0" + a.Name
		case a.Name == "export" || a.Name == "import":
			return "1" + a.Name
		case a.Destructive:
			return "3" + a.Name
		default:
			return "2" + a.Name
		}
	}
	sort.Slice(actions, func(i, j int) bool { return rank(actions[i]) < rank(actions[j]) })
}

func handleEntityAction(w http.ResponseWriter, r *http.Request, service, kind, action string) {
	spec := serviceops.EntityFor(service, kind)
	if spec == nil || kind == "databases" {
		writeDBError(w, "unknown entity kind")
		return
	}
	if action == "export" || action == "import" {
		writeDBError(w, "streaming actions use their own endpoints")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeDBError(w, "a name is required")
		return
	}
	if status, _ := podman.UnitStatus("lerd-" + service); status != "active" {
		writeDBError(w, "start the service before running entity actions")
		return
	}
	if err := serviceops.RunEntityAction(service, spec, action, strings.TrimSpace(body.Name)); err != nil {
		writeDBError(w, err.Error())
		return
	}
	writeDBOK(w)
}

// handleEntityExport streams one entity's declared export as a download.
func handleEntityExport(w http.ResponseWriter, r *http.Request, service, kind string) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if err := serviceops.ValidateDatabaseName(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if status, _ := podman.UnitStatus("lerd-" + service); status != "active" {
		http.Error(w, "start the service before exporting", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", serviceops.EntityExportFilename(service, kind, name)))
	if err := serviceops.ExportEntity(service, kind, name, w); err != nil {
		// Headers are already sent, so the browser sees a truncated file; log the
		// cause for the terminal rather than trying to rewrite the response.
		fmt.Printf("entity export failed for %s/%s/%s: %v\n", service, kind, name, err)
	}
}

// handleEntityImport loads an uploaded archive into the entity named by the
// form, streaming the body straight into the declared import the same way
// database imports do.
func handleEntityImport(w http.ResponseWriter, r *http.Request, service, kind string) {
	if status, _ := podman.UnitStatus("lerd-" + service); status != "active" {
		writeDBError(w, "start the service before importing")
		return
	}
	parts, err := r.MultipartReader()
	if err != nil {
		writeDBError(w, "an archive file is required")
		return
	}
	name := ""
	for {
		part, err := parts.NextPart()
		if err != nil {
			writeDBError(w, "an archive file is required")
			return
		}
		// The field order is the client's, so the file part is the terminator and
		// everything before it is read as a setting.
		if field := part.FormName(); field != "file" && field != "" {
			val, err := io.ReadAll(io.LimitReader(part, 1024))
			part.Close()
			if err != nil {
				writeDBError(w, "could not read the "+field+" field")
				return
			}
			if field == "name" {
				name = strings.TrimSpace(string(val))
			}
			continue
		}
		if err := serviceops.ValidateDatabaseName(name); err != nil {
			part.Close()
			writeDBError(w, err.Error())
			return
		}
		importErr := serviceops.ImportEntity(service, kind, name, part)
		// Whatever the command left unread is drained so the client still gets
		// the response instead of a reset connection.
		_, _ = io.Copy(io.Discard, part)
		part.Close()
		if importErr != nil {
			writeDBError(w, importErr.Error())
			return
		}
		writeDBOK(w)
		return
	}
}
