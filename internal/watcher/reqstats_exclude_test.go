package watcher

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/reqstats"
)

// newExcludeEnv points the ingest globals at a throwaway store and aggregator and
// restores them afterwards, so each test drives the real fan-out rather than a
// stand-in for it.
func newExcludeEnv(t *testing.T, resolve func(string) (string, bool)) *reqstats.Store {
	t.Helper()
	store, err := reqstats.OpenStore(filepath.Join(t.TempDir(), "reqstats.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	prevAgg, prevResolve, prevStore := reqAggregator, siteForHost, reqStore
	prevLastSeen, prevBuf, prevExcluded := reqLastSeen, reqBuf, reqExcluded
	t.Cleanup(func() {
		reqAggregator, siteForHost, reqStore = prevAgg, prevResolve, prevStore
		reqLastSeen, reqBuf, reqExcluded = prevLastSeen, prevBuf, prevExcluded
		store.Close()
	})
	siteForHost = resolve
	reqAggregator = reqstats.New(resolve)
	reqStore = store
	reqLastSeen = map[string]time.Time{}
	reqBuf = nil
	reqExcluded = nil
	return store
}

// storedRoutes reads the routes a key actually has rows for, with the site's
// exclusions lifted first: Recent filters excluded routes on read, so leaving
// them in place would let a row that should never have been written pass for a
// row that was correctly kept out.
func storedRoutes(t *testing.T, store *reqstats.Store, key string) []string {
	t.Helper()
	excluded, err := store.ExcludedRoutes(key)
	if err != nil {
		t.Fatalf("ExcludedRoutes: %v", err)
	}
	for _, route := range excluded {
		if err := store.UnexcludeRoute(key, route); err != nil {
			t.Fatalf("UnexcludeRoute: %v", err)
		}
	}
	recent, err := store.Recent(key, 100)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	out := make([]string, 0, len(recent))
	for _, r := range recent {
		out = append(out, r.Route)
	}
	return out
}

// An excluded route reaches neither the durable store nor the aggregator, so
// silencing a route really does stop lerd watching it rather than only hiding it
// on the way out. It still counts as traffic for the cold-start clock: a health
// check nobody wants timed is proof the site is warm all the same.
func TestIngestAccessRecord_ExcludedRouteNotRecorded(t *testing.T) {
	store := newExcludeEnv(t, func(h string) (string, bool) {
		switch h {
		case "app.test":
			return "app", true
		case "wt.test":
			return "app/feature-x", true
		}
		return "", false
	})
	if err := store.ExcludeRoute("app", "GET /health"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	refreshReqExcludes()

	ingestAccessRecord(reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/health", RequestTime: 0.04, Status: 200})
	if _, ok := reqLastSeen["app"]; !ok {
		t.Error("an excluded route left the site looking idle to the cold-start clock")
	}
	if _, ok := reqAggregator.SiteSnapshot("app"); ok {
		t.Error("an excluded route reached the aggregator")
	}

	// The exclusion is the site's, so the same route on a worktree is silent too.
	ingestAccessRecord(reqstats.AccessRecord{Host: "wt.test", Method: "GET", URI: "/health", RequestTime: 0.04, Status: 200})
	// An unrelated route on the same site still records.
	ingestAccessRecord(reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/dash", RequestTime: 0.04, Status: 200})
	flushReqStore()

	if stored := storedRoutes(t, store, "app/feature-x"); len(stored) != 0 {
		t.Errorf("stored %v on the worktree, want none: one exclusion covers every branch", stored)
	}
	if stored := storedRoutes(t, store, "app"); len(stored) != 1 || stored[0] != "GET /dash" {
		t.Errorf("stored %v, want only the unexcluded route", stored)
	}
}

// A route excluded while its requests sit in the flush buffer must not reach the
// store on the next tick, which is what makes an exclusion take effect from the
// click rather than from the tick after it.
func TestFlushReqStore_DropsBufferedExcludedRoutes(t *testing.T) {
	store := newExcludeEnv(t, func(h string) (string, bool) {
		if h == "app.test" {
			return "app", true
		}
		return "", false
	})
	ingestAccessRecord(reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/health", RequestTime: 0.04, Status: 200})
	ingestAccessRecord(reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/dash", RequestTime: 0.04, Status: 200})
	if len(reqBuf) != 2 {
		t.Fatalf("buffered %d records, want 2 before the exclusion", len(reqBuf))
	}

	if err := store.ExcludeRoute("app", "GET /health"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	refreshReqExcludes()
	flushReqStore()

	if stored := storedRoutes(t, store, "app"); len(stored) != 1 || stored[0] != "GET /dash" {
		t.Errorf("stored %v, want only GET /dash: the excluded route must never have been written", stored)
	}
}

// Putting a route back under observation must not cost the traffic already in
// the buffer. The sieve reads the set as it stands at the flush, so the seconds
// between the click and the tick are kept rather than dropped by a decision the
// user has just reversed.
func TestFlushReqStore_KeepsBufferedRowsAfterUnexclude(t *testing.T) {
	store := newExcludeEnv(t, func(h string) (string, bool) {
		if h == "app.test" {
			return "app", true
		}
		return "", false
	})
	if err := store.ExcludeRoute("app", "GET /health"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	refreshReqExcludes()

	ingestAccessRecord(reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/health", RequestTime: 0.04, Status: 200})
	if err := store.UnexcludeRoute("app", "GET /health"); err != nil {
		t.Fatalf("UnexcludeRoute: %v", err)
	}
	refreshReqExcludes()
	flushReqStore()

	if stored := storedRoutes(t, store, "app"); len(stored) != 1 || stored[0] != "GET /health" {
		t.Errorf("stored %v, want GET /health: a lifted exclusion keeps what was buffered under it", stored)
	}
}

// An id-like segment is collapsed before the exclusion is matched, so silencing
// "GET /orders/:id" covers every concrete order rather than only the one the row
// happened to show.
func TestIngestAccessRecord_ExclusionMatchesNormalizedRoute(t *testing.T) {
	store := newExcludeEnv(t, func(h string) (string, bool) {
		if h == "app.test" {
			return "app", true
		}
		return "", false
	})
	if err := store.ExcludeRoute("app", "GET /orders/:id"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	refreshReqExcludes()

	ingestAccessRecord(reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/orders/4821?tab=items", RequestTime: 0.04, Status: 200})
	flushReqStore()

	if stored := storedRoutes(t, store, "app"); len(stored) != 0 {
		t.Errorf("stored %v, want none: the concrete id is the excluded route", stored)
	}
}
