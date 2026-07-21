package ui

import (
	"encoding/json"
	"net/http"
	"strings"
)

// siteWorkerLogKinds are the /api/{kind}/{site}/logs stream routes. The kind
// doubles as the unit-name infix, so a new worker route is one entry here plus
// its mux registration.
var siteWorkerLogKinds = []string{"queue", "horizon", "schedule", "reverb", "stripe"}

// unitForLogPath maps a log stream path back to the unit behind it. Both the
// stream handlers and "follow in terminal" resolve through here, so the two can
// never disagree about which unit a pane is showing.
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
	for _, kind := range siteWorkerLogKinds {
		if rest, ok := strings.CutPrefix(path, "/api/"+kind+"/"); ok {
			parts := strings.Split(rest, "/")
			if len(parts) != 2 || parts[1] != "logs" || !allowedQueueUnit.MatchString(parts[0]) {
				return "", false
			}
			return "lerd-" + kind + "-" + parts[0], true
		}
	}
	// /api/worker/{site}/{worker}/logs — unit: lerd-{worker}-{site}
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

// handleUnitLogStream serves every per-site worker log stream, resolving the
// request path to its unit through unitForLogPath.
func handleUnitLogStream(w http.ResponseWriter, r *http.Request) {
	unit, ok := unitForLogPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	streamUnitLogs(w, r, unit)
}

// openTerminal is the seam tests replace so the handler can be exercised
// without launching a real emulator.
var openTerminal = openTerminalCommand

// handleLogTerminal opens the user's terminal emulator tailing the same unit
// the given log stream path shows, so a long-running tail can outlive the tab.
// Loopback-only, see loopbackOnlyRoutes.
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
	if err := openTerminal(logFollowScript(unit)); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "unit": unit})
}
