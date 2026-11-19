package ui

import (
	"io"
	"net/http"

	"github.com/geodro/lerd/internal/eventbus"
	"github.com/geodro/lerd/internal/podman"
)

// Seams so the handlers can be tested without a pull, a restart or the network.
var (
	updateNginxFn = podman.UpdateNginx
	checkNginxFn  = podman.CheckNginxUpdate
)

// handleNginxUpdate pulls the web server image and restarts lerd-nginx onto it
// when it moved. updated is false when nginx already ran the newest image.
func handleNginxUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	updated, err := updateNginxFn(io.Discard)
	publishStatus()
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "updated": updated})
}

// handleNginxCheck re-reads the image tag from the registry, bypassing the
// cache. A registry that cannot answer is reported as a failed check, never
// as up to date.
func handleNginxCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	available, err := checkNginxFn()
	publishStatus()
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "update_available": available})
}

func publishStatus() {
	snapshots.Invalidate(eventbus.KindStatus)
	eventbus.Default.Publish(eventbus.KindStatus)
}
