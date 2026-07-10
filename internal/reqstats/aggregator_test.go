package reqstats

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func siteResolver(m map[string]string) SiteResolver {
	return func(host string) (string, bool) {
		s, ok := m[host]
		return s, ok
	}
}

func recordN(a *Aggregator, host, method, uri string, ms float64, n int) {
	for i := 0; i < n; i++ {
		a.Record(AccessRecord{Host: host, Status: 200, RequestTime: ms / 1000, Method: method, URI: uri})
	}
}

func TestAggregatorSkipsAssetsAndZeroTime(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	recordN(a, "myapp.test", "GET", "/home", 40, 10)
	recordN(a, "myapp.test", "GET", "/build/app.js", 5, 10) // static asset
	// A zero request time is nginx answering a static file directly.
	a.Record(AccessRecord{Host: "myapp.test", Status: 200, RequestTime: 0, Method: "GET", URI: "/manifest.json"})

	snap, ok := a.SiteSnapshot("myapp")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.Samples != 10 {
		t.Errorf("samples = %d, want 10 (assets and zero-time excluded)", snap.Samples)
	}
}

// An upgraded WebSocket is logged once, at close, carrying the socket's whole
// lifetime as its request time. Recording it would plant a permanent slow route
// and drag the site median, so it must never reach a window.
func TestAggregatorSkipsWebSocketUpgrade(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	recordN(a, "myapp.test", "GET", "/home", 40, 10)
	a.Record(AccessRecord{Host: "myapp.test", Status: 101, RequestTime: 3623.181, Method: "GET", URI: "/app/cb1dxmnqqfb88d7hnchk"})

	snap, ok := a.SiteSnapshot("myapp")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.Samples != 10 {
		t.Errorf("samples = %d, want 10 (the upgrade excluded)", snap.Samples)
	}
	if len(snap.Slow) != 0 {
		t.Errorf("slow = %+v, want none: the upgrade must not be flagged", snap.Slow)
	}
	if snap.MedianMillis != 40 {
		t.Errorf("median = %v, want 40 (undragged by the upgrade)", snap.MedianMillis)
	}
}

func TestAggregatorFlagsOutlier(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	recordN(a, "myapp.test", "GET", "/home", 40, 20)
	recordN(a, "myapp.test", "GET", "/dashboard", 45, 20)
	recordN(a, "myapp.test", "GET", "/reports/7", 380, 10)

	snap, ok := a.SiteSnapshot("myapp")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if len(snap.Slow) == 0 {
		t.Fatal("expected the slow reports route to be flagged")
	}
	top := snap.Slow[0]
	if top.Route != "GET /reports/:id" {
		t.Errorf("top slow route = %q", top.Route)
	}
	if top.Multiplier < 3 {
		t.Errorf("multiplier = %v, want >= 3", top.Multiplier)
	}
}

// A route that is only sometimes slow (mostly fast redirects, a few slow renders
// under one path) has a fast median but a slow tail. Flagging on p95 must surface
// it even though its slow hits are a minority of the bucket.
func TestAggregatorFlagsTailNotMedian(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	recordN(a, "myapp.test", "GET", "/home", 40, 20)
	// Same path: 18 fast 302-style hits, 5 slow 1s renders. Median stays ~40ms.
	recordN(a, "myapp.test", "GET", "/profile/billing", 40, 18)
	recordN(a, "myapp.test", "GET", "/profile/billing", 1000, 5)

	snap, _ := a.SiteSnapshot("myapp")
	var found *RouteStat
	for i := range snap.Slow {
		if snap.Slow[i].Route == "GET /profile/billing" {
			found = &snap.Slow[i]
		}
	}
	if found == nil {
		t.Fatal("tail-slow route must be flagged on p95 even with a fast median")
	}
	if found.P95Millis < 900 {
		t.Errorf("p95 = %v, want ~1000 (the tail)", found.P95Millis)
	}
}

// A route below the sample floor must not be flagged, even if slow, so a single
// cold-cache hit doesn't light up as an anomaly.
func TestAggregatorSampleFloor(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	recordN(a, "myapp.test", "GET", "/home", 40, 20)
	recordN(a, "myapp.test", "GET", "/slow", 500, 2)

	snap, _ := a.SiteSnapshot("myapp")
	for _, r := range snap.Slow {
		if r.Route == "GET /slow" {
			t.Error("route under the sample floor must not be flagged")
		}
	}
}

// A route slow in absolute terms (p95 over a full second) must be flagged even
// below the sample floor, so a genuinely broken route a dev hits once still
// surfaces instead of hiding under the minimum-sample rule.
func TestAggregatorAbsoluteSlowFloor(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	recordN(a, "myapp.test", "GET", "/home", 40, 20)
	recordN(a, "myapp.test", "POST", "/place/search", 1200, 2)

	snap, _ := a.SiteSnapshot("myapp")
	var found *RouteStat
	for i := range snap.Slow {
		if snap.Slow[i].Route == "POST /place/search" {
			found = &snap.Slow[i]
		}
	}
	if found == nil {
		t.Fatal("a 1.2s route must be flagged even with only 2 samples")
	}
	if found.P95Millis < 1000 {
		t.Errorf("p95 = %v, want ~1200", found.P95Millis)
	}
}

// A route that was slow but whose slow samples have aged past the recency window
// must clear, so a fixed (or abandoned) route stops lingering on stale outliers.
func TestAggregatorRecentWindowDecay(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	clock := time.Unix(1_700_000_000, 0)
	a.now = func() time.Time { return clock }

	recordN(a, "myapp.test", "GET", "/home", 40, 20)
	recordN(a, "myapp.test", "POST", "/place/search", 6000, 2)

	snap, _ := a.SiteSnapshot("myapp")
	if !hasSlowRoute(snap, "POST /place/search") {
		t.Fatal("a 6s route must be flagged while its samples are recent")
	}

	// Advance past the recency window; the old slow samples fall out of scope. A
	// little fresh fast traffic keeps the site alive so the snapshot still renders.
	clock = clock.Add(recentWindow + time.Minute)
	recordN(a, "myapp.test", "GET", "/home", 40, 5)

	snap, _ = a.SiteSnapshot("myapp")
	if hasSlowRoute(snap, "POST /place/search") {
		t.Error("a route whose slow samples aged out of the recency window must clear")
	}
}

// At the route cap a new route is dropped while every existing route is still
// recent, but once the old routes age past the recency window they are evicted
// so the site keeps recording live traffic instead of wedging forever (#1).
func TestAggregatorEvictsStaleRoutesAtCap(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	clock := time.Unix(1_700_000_000, 0)
	a.now = func() time.Time { return clock }

	for i := 0; i < defaultMaxRoutes; i++ {
		a.Record(AccessRecord{Host: "myapp.test", Status: 200, RequestTime: 0.04, Method: "GET", URI: fmt.Sprintf("/old%d", i)})
	}
	// Cap is full and nothing is stale: a new slow route can't get in.
	recordN(a, "myapp.test", "GET", "/blocked", 2000, 2)
	if snap, _ := a.SiteSnapshot("myapp"); hasSlowRoute(snap, "GET /blocked") {
		t.Fatal("a new route at a full cap of live routes must be dropped")
	}

	// Age every existing route out of the recency window, then a new slow route
	// evicts the stale ones and is recorded.
	clock = clock.Add(recentWindow + time.Minute)
	recordN(a, "myapp.test", "GET", "/fresh", 2000, 2)
	if snap, _ := a.SiteSnapshot("myapp"); !hasSlowRoute(snap, "GET /fresh") {
		t.Error("a new route must be recorded once stale routes free slots under the cap")
	}
}

// A fast route must not be flagged slow just because a very frequent faster
// endpoint drags the site baseline toward zero: the relative rule requires the
// route's p95 to clear an absolute floor first (#2).
func TestAggregatorRelativeSlowFloor(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	recordN(a, "myapp.test", "GET", "/health", 5, 300)     // fast poller skews the median down
	recordN(a, "myapp.test", "GET", "/dashboard", 100, 20) // 20x the skewed baseline, but only 100ms

	snap, _ := a.SiteSnapshot("myapp")
	if hasSlowRoute(snap, "GET /dashboard") {
		t.Error("a 100ms route must not be flagged off a baseline skewed by a fast poller")
	}

	recordN(a, "myapp.test", "GET", "/reports", 400, 20) // genuinely slow, clears the floor
	snap, _ = a.SiteSnapshot("myapp")
	if !hasSlowRoute(snap, "GET /reports") {
		t.Error("a 400ms route well above baseline must still be flagged")
	}
}

func hasSlowRoute(s SiteStats, route string) bool {
	for _, r := range s.Slow {
		if r.Route == route {
			return true
		}
	}
	return false
}

// Slow must be a non-nil empty slice when nothing is flagged, so it serializes
// as [] rather than null and the UI can treat it as an array unconditionally.
func TestAggregatorSlowNeverNil(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	recordN(a, "myapp.test", "GET", "/home", 40, 20)
	snap, _ := a.SiteSnapshot("myapp")
	if snap.Slow == nil {
		t.Error("Slow must be a non-nil empty slice, got nil")
	}
	b, _ := json.Marshal(snap)
	if strings.Contains(string(b), `"slow":null`) {
		t.Errorf("Slow serialized as null: %s", b)
	}
}

func TestAggregatorMedianReported(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	recordN(a, "myapp.test", "GET", "/home", 50, 30)
	snap, _ := a.SiteSnapshot("myapp")
	if snap.MedianMillis < 45 || snap.MedianMillis > 55 {
		t.Errorf("site median = %v, want ~50", snap.MedianMillis)
	}
}

func TestAggregatorIgnoresUnknownHost(t *testing.T) {
	a := New(siteResolver(map[string]string{}))
	recordN(a, "stranger.test", "GET", "/x", 100, 10)
	if _, ok := a.SiteSnapshot("stranger"); ok {
		t.Error("unknown host must not create a site record")
	}
	if len(a.Snapshot()) != 0 {
		t.Error("unknown host must not appear in the global snapshot")
	}
}

// Distinct ids collapse onto one route so aggregation is meaningful.
func TestAggregatorGroupsByNormalizedRoute(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	recordN(a, "myapp.test", "GET", "/fast", 30, 20)
	for i := 0; i < 12; i++ {
		a.Record(AccessRecord{Host: "myapp.test", Status: 200, RequestTime: 0.3, Method: "GET", URI: "/orders/" + itoa(i)})
	}
	snap, _ := a.SiteSnapshot("myapp")
	var found *RouteStat
	for i := range snap.Slow {
		if snap.Slow[i].Route == "GET /orders/:id" {
			found = &snap.Slow[i]
		}
	}
	if found == nil {
		t.Fatal("expected grouped /orders/:id route")
	}
	if found.Samples != 12 {
		t.Errorf("grouped samples = %d, want 12", found.Samples)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
