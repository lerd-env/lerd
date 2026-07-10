package ui

import (
	"net/http"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/cleanup"
)

// diskCacheTTL bounds how often the dashboard triggers a full podman image
// scan. Computing the reclaimable total inspects every image's layers, which
// is heavier than the per-container stats poll, so the disk widget refreshes
// on its own slower cadence and this short cache collapses concurrent tabs
// into a single scan.
const diskCacheTTL = 20 * time.Second

// inspectDisk and applyDisk are the seams tests override. The dashboard button
// runs the deep scope, matching the interactive CLI default, so both the
// preview and the reclaim work off ScopeDeep.
var (
	inspectDisk = func() (cleanup.Plan, error) { return cleanup.Inspect(cleanup.ScopeDeep) }
	applyDisk   = cleanup.Apply
)

// diskImage is one reclaimable image in the preview the modal lists.
type diskImage struct {
	ID    string `json:"id"`
	Desc  string `json:"desc"`
	Bytes int64  `json:"bytes"`
}

// diskSnapshot is the reclaimable-disk preview the widget polls and the modal
// itemizes. Held is disk locked behind running containers that a restart, not
// this cleanup, would release.
type diskSnapshot struct {
	Available        bool        `json:"available"`
	ReclaimableBytes int64       `json:"reclaimable_bytes"`
	Images           []diskImage `json:"images"`
	HeldBytes        int64       `json:"held_bytes"`
	HeldCount        int         `json:"held_count"`
}

var (
	diskMu   sync.Mutex
	diskSnap *diskSnapshot
	diskAt   time.Time
)

// cachedDisk returns the reclaimable-disk preview, running a full scan at most
// once per diskCacheTTL. Holding the lock across the scan serializes the rare
// concurrent callers, which then read the fresh value, so multiple open tabs
// never fan out into parallel podman scans.
func cachedDisk() diskSnapshot {
	diskMu.Lock()
	defer diskMu.Unlock()
	if diskSnap != nil && time.Since(diskAt) < diskCacheTTL {
		return *diskSnap
	}
	snap := scanDisk()
	diskSnap = &snap
	diskAt = time.Now()
	return snap
}

// scanDisk builds the preview from a fresh deep-scope inspection. A scan error
// (podman down) reports as unavailable rather than an empty reclaim, so the
// widget can distinguish "nothing to reclaim" from "couldn't look".
func scanDisk() diskSnapshot {
	plan, err := inspectDisk()
	if err != nil {
		return diskSnapshot{Available: false}
	}
	imgs := make([]diskImage, 0, len(plan.Targets))
	for _, t := range plan.Targets {
		imgs = append(imgs, diskImage{ID: t.ID, Desc: t.Desc, Bytes: t.Bytes})
	}
	return diskSnapshot{
		Available:        true,
		ReclaimableBytes: plan.ReclaimBytes(),
		Images:           imgs,
		HeldBytes:        plan.Held.Bytes,
		HeldCount:        plan.Held.Count,
	}
}

// invalidateDiskCache drops the cached preview so the next poll reflects a
// reclaim that just ran instead of serving a stale total for up to a TTL.
func invalidateDiskCache() {
	diskMu.Lock()
	diskSnap = nil
	diskMu.Unlock()
}

// handleDisk serves the reclaimable-disk preview (GET) and runs the reclaim
// (POST). The POST is loopback-only: the deep scope removes images on the host,
// including dangling ones from other podman workloads, so it stays off the LAN
// even when remote control is on.
func handleDisk(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, cachedDisk())
	case http.MethodPost:
		if !isLoopbackRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		handleDiskCleanup(w)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDiskCleanup re-inspects the host and applies that fresh plan rather
// than trusting anything the client posted, so a modal left open across a
// rebuild can't ask podman to remove an image that has since become live.
func handleDiskCleanup(w http.ResponseWriter) {
	plan, err := inspectDisk()
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	removed, reclaimed := applyDisk(plan)
	invalidateDiskCache()
	writeJSON(w, map[string]any{"ok": true, "removed": removed, "reclaimed_bytes": reclaimed})
}
