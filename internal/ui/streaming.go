package ui

import (
	"encoding/json"
	"net/http"

	"github.com/geodro/lerd/internal/config"
)

// streamingState reports whether streaming mode is on and which sites it hides.
func streamingState(cfg *config.GlobalConfig) (bool, map[string]bool) {
	if cfg == nil || !cfg.UI.StreamingMode {
		return false, map[string]bool{}
	}
	reg, err := config.LoadSites()
	if err != nil {
		return true, map[string]bool{}
	}
	return true, cfg.StreamingHidden(reg)
}

// privateWorkspaceNames lists the workspaces flagged private, so the dashboard
// can label the toggle on each one. While streaming they are hidden anyway.
func privateWorkspaceNames(cfg *config.GlobalConfig) []string {
	names := []string{}
	if cfg == nil || cfg.UI.StreamingMode {
		return names
	}
	for _, w := range cfg.Workspaces {
		if w.Private {
			names = append(names, w.Name)
		}
	}
	return names
}

// handleSettingsStreaming turns streaming mode on or off. The sites and status
// snapshots rebuild after it, so every open dashboard drops or regains the
// private sites and workspaces at once.
func handleSettingsStreaming(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	if body.Enabled {
		broker.broadcastStreamingOn()
	}
	if err := config.SetStreamingMode(body.Enabled); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "enabled": body.Enabled})
}
