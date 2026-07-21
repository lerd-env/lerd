package ui

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/stats"
)

// The dashboard poll and the server cache are two constants in two languages
// with nothing tying them together, and they drifted: a 5s poll against a 3s
// TTL missed the cache on every single request, so a ~2s streaming
// `podman stats` ran back to back for as long as a tab was open. The mirror is
// only worth anything if it is checked against the store itself.
func TestStatsClientPollMatchesStore(t *testing.T) {
	src, err := os.ReadFile("web/src/stores/stats.ts")
	if err != nil {
		t.Fatalf("read stats store: %v", err)
	}
	m := regexp.MustCompile(`POLL_INTERVAL_MS\s*=\s*(\d+)`).FindSubmatch(src)
	if m == nil {
		t.Fatal("POLL_INTERVAL_MS not found in stores/stats.ts")
	}
	ms, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("parse POLL_INTERVAL_MS: %v", err)
	}
	if got := time.Duration(ms) * time.Millisecond; got != statsClientPollInterval {
		t.Fatalf("stores/stats.ts polls every %v, statsClientPollInterval says %v", got, statsClientPollInterval)
	}
}

// The client must poll faster than the TTL, so that the TTL is what governs
// how often the podman cost is actually paid.
func TestStatsCacheTTLOutlivesClientPoll(t *testing.T) {
	if stats.CacheTTL <= statsClientPollInterval {
		t.Fatalf("stats TTL %v must exceed the %v client poll, otherwise every poll is a cache miss",
			stats.CacheTTL, statsClientPollInterval)
	}
}
