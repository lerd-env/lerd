package ui

import "testing"

// The dashboard poll and the server cache are two constants in two languages
// with nothing tying them together, and they drifted: a 5s poll against a 3s
// TTL missed the cache on every single request, so a ~2s streaming
// `podman stats` ran back to back for as long as a tab was open.
//
// The client must poll faster than the TTL, so that the TTL is what governs
// how often the podman cost is actually paid.
func TestStatsCacheTTLOutlivesClientPoll(t *testing.T) {
	if statsCacheTTL <= statsClientPollInterval {
		t.Fatalf("stats TTL %v must exceed the %v client poll, otherwise every poll is a cache miss",
			statsCacheTTL, statsClientPollInterval)
	}
}
