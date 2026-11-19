package ui

import (
	"io"
	"net/http"
	"time"

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
	if !nginxActionAllowed(w, r) {
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
	if !nginxActionAllowed(w, r) {
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

// nginxActionAllowed admits a POST from a caller with host-action authority:
// pulling and restarting nginx is a host action, so a LAN session needs full access.
func nginxActionAllowed(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	if !hasHostActionAuthority(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

// watchNginxImage re-reads the nginx update state every hour. Nothing else
// rebuilds status on an idle machine, and the read is what fetches an expired
// digest in the background and announces a republished image.
func watchNginxImage() {
	for range time.Tick(time.Hour) {
		podman.NginxUpdateAvailable()
	}
}

func publishStatus() {
	snapshots.Invalidate(eventbus.KindStatus)
	eventbus.Default.Publish(eventbus.KindStatus)
}
