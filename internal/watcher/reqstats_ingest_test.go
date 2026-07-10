package watcher

import (
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

// A WebSocket upgrade reaches neither the aggregator nor the durable store: its
// request time is the socket's lifetime, not work the app did.
func TestIngestAccessRecord_WebSocketUpgradeNotRecorded(t *testing.T) {
	resolve := func(h string) (string, bool) {
		if h == "app.test" {
			return "app", true
		}
		return "", false
	}
	prevAgg, prevResolve, prevStore := reqAggregator, siteForHost, reqStore
	prevLastSeen, prevBuf := reqLastSeen, reqBuf
	t.Cleanup(func() {
		reqAggregator, siteForHost, reqStore = prevAgg, prevResolve, prevStore
		reqLastSeen, reqBuf = prevLastSeen, prevBuf
	})
	siteForHost = resolve
	reqAggregator = reqstats.New(resolve)
	reqStore = nil
	reqLastSeen = map[string]time.Time{}
	reqBuf = nil

	ingestAccessRecord(reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/dash", RequestTime: 0.04, Status: 200})
	ingestAccessRecord(reqstats.AccessRecord{Host: "app.test", Method: "GET", URI: "/app/cb1dxmnqqfb88d7hnchk", RequestTime: 3623.181, Status: 101})

	snap, ok := reqAggregator.SiteSnapshot("app")
	if !ok {
		t.Fatal("aggregator should have the page request")
	}
	if snap.Samples != 1 {
		t.Errorf("aggregator samples = %d, want 1 (the upgrade excluded)", snap.Samples)
	}
	if len(snap.Slow) != 0 {
		t.Errorf("slow = %+v, want none", snap.Slow)
	}
	// The upgrade must not update the cold-start clock either: it isn't a request
	// the site served, so it can't make the next real request look warm.
	if _, seen := reqLastSeen["app"]; !seen {
		t.Fatal("expected the page request to seed the cold-start clock")
	}
}
