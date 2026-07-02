package reqstats

import (
	"encoding/json"
	"strings"
	"testing"
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
