package ui

import (
	"net/http"
	"time"

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
	writeJSON(w, stats.Cached(stats.CacheTTL))
}
