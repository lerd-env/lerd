package ui

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/geodro/lerd/internal/config"
)

// handleSettingsSetup records where the first-run checklist stands, then tells
// every open dashboard, so the browser and the desktop app never disagree about
// whether setup is still showing.
func handleSettingsSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	if body.State != "active" && body.State != "done" {
		writeJSON(w, map[string]any{"ok": false, "error": fmt.Sprintf("unknown setup state %q", body.State)})
		return
	}
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "loading config"})
		return
	}
	cfg.UI.Setup = body.State
	if err := config.SaveGlobal(cfg); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	broker.broadcastSetup()
	writeJSON(w, map[string]any{"ok": true, "state": body.State})
}
