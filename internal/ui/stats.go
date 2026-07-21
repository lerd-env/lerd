package ui

import (
	"net/http"
	"time"

	"github.com/geodro/lerd/internal/stats"
)

// statsCacheTTL is the freshness window the dashboard polls against, and
// statsClientPollInterval mirrors POLL_INTERVAL_MS in stores/stats.ts.
//
// stats.Read streams `podman stats --interval 1` and costs roughly two
// seconds, not the ~100-300ms an earlier `--no-stream` implementation took.
// The client polls faster than the TTL on purpose, so repeat polls share one
// value and the TTL alone decides how often that two second cost is paid. A
// TTL below the poll interval means every request misses and the stream runs
// essentially back to back against the podman VM for as long as a tab is open.
const (
	statsCacheTTL           = 10 * time.Second
	statsClientPollInterval = 5 * time.Second
)

// handleStats returns the latest container stats via the shared
// internal/stats cache. Kept as a thin wrapper so the JSON shape exposed
// to the web UI stays stable while the parsing logic lives in one place.
func handleStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, stats.Cached(statsCacheTTL))
}
