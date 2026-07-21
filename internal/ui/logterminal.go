package ui

import (
	"encoding/json"
	"net/http"
	"strings"
)

// unitForLogPath maps a log stream path the UI is subscribed to back to the
// unit behind it, so "follow in terminal" can reuse whatever the log pane is
// already showing without every caller having to know its own unit name. The
// mapping mirrors the stream handlers in server.go.
func unitForLogPath(path string) (string, bool) {
	if path == "/api/watcher/logs" {
		return "lerd-watcher", true
	}
	if rest, ok := strings.CutPrefix(path, "/api/logs/"); ok {
		if !allowedContainer.MatchString(rest) {
			return "", false
		}
		return rest, true
	}
	// /api/{kind}/{site}/logs and /api/worker/{site}/{worker}/logs
	for _, kind := range []string{"queue", "horizon", "schedule", "reverb", "stripe"} {
		if rest, ok := strings.CutPrefix(path, "/api/"+kind+"/"); ok {
			parts := strings.Split(rest, "/")
			if len(parts) != 2 || parts[1] != "logs" || !allowedQueueUnit.MatchString(parts[0]) {
				return "", false
			}
			return "lerd-" + kind + "-" + parts[0], true
		}
	}
	if rest, ok := strings.CutPrefix(path, "/api/worker/"); ok {
		parts := strings.Split(rest, "/")
		if len(parts) != 3 || parts[2] != "logs" ||
			!allowedQueueUnit.MatchString(parts[0]) || !allowedQueueUnit.MatchString(parts[1]) {
			return "", false
		}
		return "lerd-" + parts[1] + "-" + parts[0], true
	}
	return "", false
}

// handleLogTerminal opens the user's terminal emulator tailing the same unit
// the given log stream path shows, so a long-running tail can outlive the tab.
func handleLogTerminal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	unit, ok := unitForLogPath(body.Path)
	if !ok {
		http.Error(w, "unknown log stream", http.StatusNotFound)
		return
	}
	if err := openTerminalCommand(logFollowScript(unit)); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "unit": unit})
}
