package watcher

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/reqstats"
)

// A cold start (the first request after the site sat idle past coldGap) must not
// reach the live aggregator, so the slow-route notifier and the doctor never fire
// on a wake's inflated time. A warm request still counts.
func TestIngestAccessRecord_ColdStartExcludedFromAggregator(t *testing.T) {
	resolve := func(h string) (string, bool) {
		if h == "app.test" {
			return "app", true
		}
		return "", false
	}
	prevAgg, prevResolve, prevStore := reqAggregator, siteForHost, reqStore
	prevLastSeen, prevGap := reqLastSeen, coldGap
	t.Cleanup(func() {
		reqAggregator, siteForHost, reqStore = prevAgg, prevResolve, prevStore
		reqLastSeen, coldGap = prevLastSeen, prevGap
	})
	siteForHost = resolve
	reqAggregator = reqstats.New(resolve)
	reqStore = nil
	reqLastSeen = map[string]time.Time{}
	coldGap = 30 * time.Minute

	rec := reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/dash", RequestTime: 1.5, Status: 200}

	ingestAccessRecord(rec) // warm: first ever request, not a cold start
	// Back-date the site's last-seen so the next request lands past coldGap.
	reqLastSeen["app"] = time.Now().Add(-time.Hour)
	ingestAccessRecord(rec) // cold: must be dropped from the aggregator

	snap, ok := reqAggregator.SiteSnapshot("app")
	if !ok {
		t.Fatal("aggregator should have the warm sample")
	}
	if snap.Samples != 1 {
		t.Fatalf("aggregator samples = %d, want 1 (the cold start excluded)", snap.Samples)
	}
}

// A WebSocket upgrade reaches neither the durable store nor the cold-start clock:
// its request time is the socket's lifetime, not work the app did. The aggregator
// filters upgrades itself, so what only this fan-out decides is the store write
// and the clock, and those are what this asserts.
func TestIngestAccessRecord_WebSocketUpgradeNotRecorded(t *testing.T) {
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
	prevLastSeen, prevBuf := reqLastSeen, reqBuf
	t.Cleanup(func() {
		reqAggregator, siteForHost, reqStore = prevAgg, prevResolve, prevStore
		reqLastSeen, reqBuf = prevLastSeen, prevBuf
		store.Close()
	})
	siteForHost = resolve
	reqAggregator = reqstats.New(resolve)
	reqStore = store
	reqLastSeen = map[string]time.Time{}
	reqBuf = nil

	ingestAccessRecord(reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/dash", RequestTime: 0.04, Status: 200})
	if len(reqBuf) != 1 {
		t.Fatalf("buffered %d records, want the page request", len(reqBuf))
	}

	// Back-date the clock to a value the upgrade must leave untouched.
	warmAt := time.Now().Add(-time.Hour)
	reqLastSeen["app"] = warmAt
	ingestAccessRecord(reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/app/cb1dxmnqqfb88d7hnchk", RequestTime: 3623.181, Status: 101})

	if len(reqBuf) != 1 {
		t.Errorf("buffered %d records, want 1: the upgrade must not be stored", len(reqBuf))
	}
	if !reqLastSeen["app"].Equal(warmAt) {
		t.Error("the upgrade advanced the cold-start clock, so the next real request would count as warm")
	}
	snap, ok := reqAggregator.SiteSnapshot("app")
	if !ok {
		t.Fatal("aggregator should have the page request")
	}
	if snap.Samples != 1 {
		t.Errorf("aggregator samples = %d, want 1 (the upgrade excluded)", snap.Samples)
	}
}
