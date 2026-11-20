package ui

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/geodro/lerd/internal/cfgedit"
	"github.com/geodro/lerd/internal/siteops"
)

// handleWebRoutes serves the global settings of lerd's own web server under
// /api/web/, in the same shapes as /api/nginx/ so the dashboard's config
// editor drives either. Nothing reloads: the server rereads the file.
func handleWebRoutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/web/")
	switch {
	case rest == "config" && r.Method == http.MethodGet:
		got, err := siteops.ReadGlobalWeb()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, NginxConfigReadResponse{Path: got.Path, Content: got.Body, Exists: got.Exists})
	case rest == "config" && r.Method == http.MethodPost:
		var req NginxConfigWriteRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
			writeJSON(w, NginxConfigWriteResponse{OK: false, Error: "invalid body: " + err.Error()})
			return
		}
		res, err := siteops.SaveGlobalWeb(req.Content, req.Backup)
		if err != nil {
			writeJSON(w, NginxConfigWriteResponse{OK: false, Error: err.Error()})
			return
		}
		writeJSON(w, NginxConfigWriteResponse(res))
	case rest == "backups" && r.Method == http.MethodGet:
		list, err := siteops.ListGlobalWebBackups()
		if err != nil {
			http.Error(w, "listing backups: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if list == nil {
			list = []cfgedit.Backup{}
		}
		writeJSON(w, list)
	case strings.HasPrefix(rest, "backups/") && r.Method == http.MethodGet:
		data, err := siteops.ReadGlobalWebBackup(strings.TrimPrefix(rest, "backups/"))
		writeBackupContent(w, r, data, err)
	case rest == "reset" && r.Method == http.MethodPost:
		if err := siteops.ResetGlobalWeb(); err != nil {
			writeJSON(w, NginxConfigResetResponse{OK: false, Error: err.Error()})
			return
		}
		writeJSON(w, NginxConfigResetResponse{OK: true})
	case rest == "restore" && r.Method == http.MethodPost:
		var req NginxConfigRestoreRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
			writeJSON(w, NginxConfigRestoreResponse{OK: false, Error: "invalid body: " + err.Error()})
			return
		}
		if !siteops.ValidGlobalWebBackupName(req.Name) {
			writeJSON(w, NginxConfigRestoreResponse{OK: false, Error: "invalid backup name"})
			return
		}
		res, err := siteops.RestoreGlobalWeb(req.Name)
		if err != nil {
			writeJSON(w, NginxConfigRestoreResponse{OK: false, Error: err.Error()})
			return
		}
		writeJSON(w, NginxConfigRestoreResponse(res))
	default:
		http.NotFound(w, r)
	}
}
