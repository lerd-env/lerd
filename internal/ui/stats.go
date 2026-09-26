package ui

import (
	"net/http"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/stats"
)

// statsClientPollInterval mirrors POLL_INTERVAL_MS in stores/stats.ts. It has
// to stay below stats.CacheTTL so consecutive polls share one cached value and
// the TTL alone decides how often the ~2s `podman stats` stream is paid for.
const statsClientPollInterval = 5 * time.Second

// handleStats returns the latest container stats via the shared
// internal/stats cache. Kept as a thin wrapper so the JSON shape exposed
// to the web UI stays stable while the parsing logic lives in one place.
func handleStats(w http.ResponseWriter, _ *http.Request) {
	hidden, _ := streamingHiddenNow()
	writeJSON(w, markOrphans(stats.WithoutSites(stats.Cached(stats.CacheTTL), hidden), statsOrphaned))
}

// statsOrphaned is the seam markOrphans asks through.
var statsOrphaned = serviceops.ServiceOrphaned

// markOrphans flags the service containers nothing installed stands behind, so
// the dashboard can offer to remove them. It copies the rows: the snapshot is
// the shared cached one.
func markOrphans(snap stats.Snapshot, orphaned func(string) bool) stats.Snapshot {
	rows := make([]stats.ContainerStat, len(snap.Containers))
	for i, c := range snap.Containers {
		name, ok := strings.CutPrefix(c.Name, "lerd-")
		c.Orphaned = ok && orphaned(name)
		rows[i] = c
	}
	snap.Containers = rows
	return snap
}
