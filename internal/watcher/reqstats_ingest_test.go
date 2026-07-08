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
