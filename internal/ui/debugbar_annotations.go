package ui

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/geodro/lerd/internal/annotations"
)

// serveDebugbarAnnotations is the bar's annotations, one site's: GET lists the
// open ones, POST makes one, POST {id} changes its comment or status and
// DELETE {id} forgets it.
func serveDebugbarAnnotations(w http.ResponseWriter, r *http.Request, site, rest string) {
	switch {
	case rest == "" && r.Method == http.MethodGet:
		list, err := annotations.List(site, annotations.StatusOpen)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, list)
	case rest == "" && r.Method == http.MethodPost:
		var a annotations.Annotation
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&a); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		a.Site = site
		created, err := annotations.Add(a)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, created)
	case rest != "" && r.Method == http.MethodPost:
		var req struct {
			Comment string `json:"comment"`
			Status  string `json:"status"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		var a annotations.Annotation
		var err error
		switch req.Status {
		case "":
			a, err = annotations.Update(site, rest, req.Comment)
		case annotations.StatusResolved:
			a, err = annotations.Resolve(site, rest, "")
		case annotations.StatusOpen:
			a, err = annotations.Reopen(site, rest)
		default:
			http.Error(w, "unknown status "+req.Status, http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, a)
	case rest != "" && r.Method == http.MethodDelete:
		if err := annotations.Delete(site, rest); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAnnotations serves /api/annotations: GET ?site=&status= lists a site's
// notes (open by default, status=all for every one), GET /{site}/{id} reads one
// and POST /{site}/{id} {status, resolution} resolves or reopens it.
func handleAnnotations(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/annotations"), "/")
	if rest == "" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status := r.URL.Query().Get("status")
		switch status {
		case "":
			status = annotations.StatusOpen
		case "all":
			status = ""
		}
		list, err := annotations.List(r.URL.Query().Get("site"), status)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, list)
		return
	}
	site, id, _ := strings.Cut(rest, "/")
	switch r.Method {
	case http.MethodGet:
		a, err := annotations.Get(site, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, a)
	case http.MethodPost:
		if !hasHostActionAuthority(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var req struct {
			Status     string `json:"status"`
			Resolution string `json:"resolution"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		var a annotations.Annotation
		var err error
		switch req.Status {
		case annotations.StatusResolved:
			a, err = annotations.Resolve(site, id, req.Resolution)
		case annotations.StatusOpen:
			a, err = annotations.Reopen(site, id)
		default:
			http.Error(w, "status must be resolved or open", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, a)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
