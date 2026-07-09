package ui

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteinfo"
)

// resolveSiteWorkspace reports the workspace a site renders under. A group
// secondary follows its main, so the sidebar never splits a group across two
// sections. groupMainName maps a group key to its main site's name.
func resolveSiteWorkspace(e siteinfo.EnrichedSite, groupMainName, siteWorkspace map[string]string) string {
	if e.GroupSubdomain != "" {
		if main, ok := groupMainName[e.Group]; ok {
			return siteWorkspace[main]
		}
	}
	return siteWorkspace[e.Name]
}

// Workspaces are a display-only grouping of sites kept in the global config.
// Nothing here touches a site's serving setup; see internal/config/workspaces.go.

type WorkspaceResponse struct {
	Name  string   `json:"name"`
	Sites []string `json:"sites"`
}

type workspaceCreateRequest struct {
	Name string `json:"name"`
}

type workspaceRenameRequest struct {
	Old string `json:"old"`
	New string `json:"new"`
}

type workspaceAssignRequest struct {
	Sites     []string `json:"sites"`
	Workspace string   `json:"workspace"`
	Create    bool     `json:"create"`
}

// workspaceLayoutRequest replaces the whole workspace list, so one sidebar drag
// persists both the workspace order and the membership in a single write.
// SiteOrder is optional and only sent when the drag also changed the manual
// site order, so a cosmetic move never rewrites sites.yaml.
type workspaceLayoutRequest struct {
	Workspaces []WorkspaceResponse `json:"workspaces"`
	SiteOrder  []string            `json:"site_order"`
}

// handleWorkspaces serves GET /api/workspaces (list) and POST /api/workspaces (create).
func handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := config.ListWorkspaces()
		if err != nil {
			writeJSON(w, SiteActionResponse{Error: err.Error()})
			return
		}
		out := make([]WorkspaceResponse, 0, len(list))
		for _, ws := range list {
			sites := ws.Sites
			if sites == nil {
				sites = []string{}
			}
			out = append(out, WorkspaceResponse{Name: ws.Name, Sites: sites})
		}
		writeJSON(w, out)
	case http.MethodPost:
		var req workspaceCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, SiteActionResponse{Error: "invalid request body"})
			return
		}
		writeWorkspaceResult(w, config.AddWorkspace(req.Name))
	default:
		http.NotFound(w, r)
	}
}

// handleWorkspaceRoutes dispatches /api/workspaces/{rename,delete,assign,layout}.
func handleWorkspaceRoutes(w http.ResponseWriter, r *http.Request) {
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/workspaces/"), "/")
	switch action {
	case "rename":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var req workspaceRenameRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, SiteActionResponse{Error: "invalid request body"})
			return
		}
		writeWorkspaceResult(w, config.RenameWorkspace(req.Old, req.New))
	case "delete":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var req workspaceCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, SiteActionResponse{Error: "invalid request body"})
			return
		}
		writeWorkspaceResult(w, config.DeleteWorkspace(req.Name))
	case "assign":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var req workspaceAssignRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, SiteActionResponse{Error: "invalid request body"})
			return
		}
		writeWorkspaceResult(w, config.AssignSiteWorkspace(req.Sites, req.Workspace, req.Create))
	case "layout":
		if r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		var req workspaceLayoutRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, SiteActionResponse{Error: "invalid request body"})
			return
		}
		writeWorkspaceResult(w, applyWorkspaceLayout(req))
	default:
		http.NotFound(w, r)
	}
}

// applyWorkspaceLayout writes the config first, then the registry order. If the
// registry write fails the config is restored, so the client has a single
// failure to roll back rather than a half-applied pair.
func applyWorkspaceLayout(req workspaceLayoutRequest) error {
	prev, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	if err := config.SetWorkspaceLayout(mergeWorkspaceLayout(req.Workspaces, prev.Workspaces)); err != nil {
		return err
	}
	if len(req.SiteOrder) == 0 {
		return nil
	}
	if err := config.ReorderSites(req.SiteOrder); err != nil {
		if restoreErr := config.SetWorkspaceLayout(prev.Workspaces); restoreErr != nil {
			return restoreErr
		}
		return err
	}
	return nil
}

// mergeWorkspaceLayout puts the client's workspaces in the order it sent them,
// then appends any the client didn't know about, keeping their members. A
// layout built from a stale snapshot therefore reorders and reassigns without
// deleting a workspace someone created meanwhile; deletion has its own route.
// SetWorkspaceLayout keeps a site's first listing, so a move wins over the
// workspace the site is being moved out of.
func mergeWorkspaceLayout(sent []WorkspaceResponse, existing []config.Workspace) []config.Workspace {
	named := make(map[string]bool, len(sent))
	out := make([]config.Workspace, 0, len(sent)+len(existing))
	for _, ws := range sent {
		named[strings.TrimSpace(ws.Name)] = true
		out = append(out, config.Workspace{Name: ws.Name, Sites: ws.Sites})
	}
	for _, ws := range existing {
		if !named[ws.Name] {
			out = append(out, ws)
		}
	}
	return out
}

func writeWorkspaceResult(w http.ResponseWriter, err error) {
	if err != nil {
		writeJSON(w, SiteActionResponse{Error: err.Error()})
		return
	}
	writeJSON(w, SiteActionResponse{OK: true})
}
