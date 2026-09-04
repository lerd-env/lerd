package watcher

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/reqstats"
)

// An excluded route reaches nothing: not the durable store, not the aggregator,
// and not the cold-start clock, so silencing a route really does stop lerd
// watching it rather than only hiding it on the way out.
func TestIngestAccessRecord_ExcludedRouteNotRecorded(t *testing.T) {
	resolve := func(h string) (string, bool) {
		switch h {
		case "app.test":
			return "app", true
		case "wt.test":
			return "app/feature-x", true
		}
		return "", false
	}
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

	if err := store.ExcludeRoute("app", "GET /health"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	refreshReqExcludes()

	ingestAccessRecord(reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/health", RequestTime: 0.04, Status: 200})
	if len(reqBuf) != 0 {
		t.Errorf("buffered %d records, want none for an excluded route", len(reqBuf))
	}
	if _, ok := reqLastSeen["app"]; ok {
		t.Error("an excluded route advanced the cold-start clock")
	}
	if _, ok := reqAggregator.SiteSnapshot("app"); ok {
		t.Error("an excluded route reached the aggregator")
	}

	// The exclusion is the site's, so the same route on a worktree is silent too.
	ingestAccessRecord(reqstats.AccessRecord{Host: "wt.test", Method: "GET", URI: "/health", RequestTime: 0.04, Status: 200})
	if len(reqBuf) != 0 {
		t.Errorf("buffered %d records, want none on the worktree either", len(reqBuf))
	}

	// An unrelated route on the same site still records.
	ingestAccessRecord(reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/dash", RequestTime: 0.04, Status: 200})
	if len(reqBuf) != 1 {
		t.Errorf("buffered %d records, want the unexcluded route", len(reqBuf))
	}
}

// A route excluded while its requests sit in the flush buffer must not reach the
// store on the next tick: the ingest gate reads a cache refreshed on that same
// tick, so the buffer always holds traffic taken in before the exclusion existed.
func TestFlushReqStore_DropsBufferedExcludedRoutes(t *testing.T) {
	resolve := func(h string) (string, bool) {
		if h == "app.test" {
			return "app", true
		}
		return "", false
	}
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

	// Lift the exclusion before reading, so the assertion is about what reached the
	// store rather than what the read filter is hiding.
	if err := store.UnexcludeRoute("app", "GET /health"); err != nil {
		t.Fatalf("UnexcludeRoute: %v", err)
	}
	recent, err := store.Recent("app", 20)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 1 || recent[0].Route != "GET /dash" {
		t.Errorf("stored %+v, want only GET /dash: the excluded route must never have been written", recent)
	}
}

// An id-like segment is collapsed before the exclusion is matched, so silencing
// "GET /orders/:id" covers every concrete order rather than only the one the row
// happened to show.
func TestIngestAccessRecord_ExclusionMatchesNormalizedRoute(t *testing.T) {
	resolve := func(h string) (string, bool) {
		if h == "app.test" {
			return "app", true
		}
		return "", false
	}
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

	if err := store.ExcludeRoute("app", "GET /orders/:id"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	refreshReqExcludes()

	ingestAccessRecord(reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/orders/4821?tab=items", RequestTime: 0.04, Status: 200})
	if len(reqBuf) != 0 {
		t.Errorf("buffered %d records, want none: the concrete id is the excluded route", len(reqBuf))
	}
}
