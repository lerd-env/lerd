package ui

import "testing"

// Go randomises map iteration, so a project carrying more than one DSN against
// the same engine would be attributed to a different database on every
// snapshot. The walk has to be ordered.
func TestDSNDatabaseForIsStableAcrossRuns(t *testing.T) {
	vals := map[string]string{
		"QUEUE_DSN":     "mongodb://root:lerd@lerd-mongo:27017/jobs?authSource=admin",
		"ANALYTICS_DSN": "mongodb://root:lerd@lerd-mongo:27017/metrics",
		"CACHE_URL":     "redis://lerd-redis:6379/0",
		"APP_NAME":      "shop",
	}
	first := dsnDatabaseFor(vals, "mongo")
	if first == "" {
		t.Fatal("no database resolved from the DSNs")
	}
	// The alphabetically first key wins, so the answer is predictable rather
	// than merely repeatable.
	if first != "metrics" {
		t.Errorf("resolved %q, want metrics (ANALYTICS_DSN sorts first)", first)
	}
	for range 200 {
		if got := dsnDatabaseFor(vals, "mongo"); got != first {
			t.Fatalf("resolution is not stable: got %q then %q", first, got)
		}
	}
}

func TestDSNDatabaseForIgnoresOtherServices(t *testing.T) {
	vals := map[string]string{"CACHE_URL": "redis://lerd-redis:6379/0"}
	if got := dsnDatabaseFor(vals, "mongo"); got != "" {
		t.Errorf("resolved %q from another service's DSN", got)
	}
}
