package reqstats

import (
	"path/filepath"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "reqstats.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// base is a fixed clock so tests never touch the wall clock.
var base = time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)

func seed(t *testing.T, s *Store, recs []Record) {
	t.Helper()
	if err := s.Insert(recs); err != nil {
		t.Fatalf("Insert: %v", err)
	}
}

func mk(n int, offset time.Duration, site, method, route, uri string, status int, ms float64) []Record {
	out := make([]Record, n)
	for i := 0; i < n; i++ {
		out[i] = Record{
			At:   base.Add(offset + time.Duration(i)*time.Second),
			Site: site, Method: method, Route: route, URI: uri, Status: status, Millis: ms,
		}
	}
	return out
}

func TestStoreAnalyticsAggregates(t *testing.T) {
	s := tempStore(t)
	var recs []Record
	recs = append(recs, mk(10, 0, "app", "GET", "GET /fast", "/fast", 200, 20)...)
	recs = append(recs, mk(10, time.Minute, "app", "GET", "GET /slow", "/slow", 200, 200)...)
	recs = append(recs, mk(2, 2*time.Minute, "app", "GET", "GET /boom", "/boom", 500, 30)...)
	seed(t, s, recs)

	a, err := s.SiteAnalytics("app", base.Add(-time.Hour), base.Add(time.Hour))
	if err != nil {
		t.Fatalf("SiteAnalytics: %v", err)
	}
	if a.Samples != 22 {
		t.Errorf("samples = %d, want 22", a.Samples)
	}
	if a.Status.C2xx != 20 || a.Status.C5xx != 2 {
		t.Errorf("status = %+v, want 20x2xx 2x5xx", a.Status)
	}
	if len(a.Routes) != 3 {
		t.Fatalf("routes = %d, want 3", len(a.Routes))
	}
	byRoute := map[string]RouteStat{}
	for _, r := range a.Routes {
		byRoute[r.Route] = r
	}
	if r := byRoute["GET /slow"]; r.P50Millis < 180 || r.Samples != 10 {
		t.Errorf("/slow = %+v, want p50 ~200 samples 10", r)
	}
	total := 0
	for _, b := range a.Distribution {
		total += b.Count
	}
	if total != 22 {
		t.Errorf("distribution total = %d, want 22", total)
	}
	// 20ms and 30ms land in <50 bins, 200ms lands in 100–250.
	if a.Distribution[len(a.Distribution)-1].UpperMillis != 0 {
		t.Errorf("top bucket should be open-ended (UpperMillis 0)")
	}
}

func TestStoreAnalyticsExcludesColdFromTiming(t *testing.T) {
	s := tempStore(t)
	recs := mk(10, 0, "app", "GET", "GET /x", "/x", 200, 20)
	cold := mk(1, time.Minute, "app", "GET", "GET /x", "/x", 200, 3000)[0]
	cold.Cold = true
	recs = append(recs, cold)
	seed(t, s, recs)

	a, err := s.SiteAnalytics("app", base.Add(-time.Hour), base.Add(time.Hour))
	if err != nil {
		t.Fatalf("SiteAnalytics: %v", err)
	}
	if a.Samples != 11 {
		t.Errorf("samples = %d, want 11 (cold counted)", a.Samples)
	}
	if a.ColdStarts != 1 {
		t.Errorf("cold starts = %d, want 1", a.ColdStarts)
	}
	if a.P95Millis > 100 {
		t.Errorf("p95 = %v, the 3000ms cold start must be excluded from timing", a.P95Millis)
	}
	if len(a.Routes) != 1 {
		t.Fatalf("routes = %d, want 1", len(a.Routes))
	}
	r := a.Routes[0]
	if r.P95Millis > 100 {
		t.Errorf("route p95 = %v, cold start must be excluded", r.P95Millis)
	}
	if r.Samples != 11 {
		t.Errorf("route samples = %d, want 11 (cold counted)", r.Samples)
	}
	total := 0
	for _, b := range a.Distribution {
		total += b.Count
	}
	if total != 10 {
		t.Errorf("distribution total = %d, want 10 (warm only)", total)
	}
}

func TestStoreAnalyticsRespectsRange(t *testing.T) {
	s := tempStore(t)
	seed(t, s, mk(5, -2*time.Hour, "app", "GET", "GET /old", "/old", 200, 20))
	seed(t, s, mk(5, 0, "app", "GET", "GET /new", "/new", 200, 20))

	a, err := s.SiteAnalytics("app", base.Add(-time.Hour), base.Add(time.Hour))
	if err != nil {
		t.Fatalf("SiteAnalytics: %v", err)
	}
	if a.Samples != 5 {
		t.Errorf("samples = %d, want 5 (old rows excluded by range)", a.Samples)
	}
}

func TestStoreRecentOrdersNewestFirst(t *testing.T) {
	s := tempStore(t)
	seed(t, s, mk(3, 0, "app", "GET", "GET /a", "/a", 200, 20))
	seed(t, s, mk(1, time.Hour, "app", "POST", "POST /b", "/b/9", 201, 42))

	recent, err := s.Recent("app", 2)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("recent = %d, want 2 (limited)", len(recent))
	}
	if recent[0].URI != "/b/9" {
		t.Errorf("newest = %q, want /b/9", recent[0].URI)
	}
}

func TestRecordFromNormalizes(t *testing.T) {
	rec := RecordFrom(AccessRecord{
		Host: "app.test", Status: 200, RequestTime: 0.15, Method: "get", URI: "/products/42?ref=x",
	}, "app", base)
	if rec.Route != "GET /products/:id" {
		t.Errorf("route = %q, want GET /products/:id", rec.Route)
	}
	if rec.Method != "GET" {
		t.Errorf("method = %q, want GET (upper, trimmed)", rec.Method)
	}
	if rec.URI != "/products/42" {
		t.Errorf("uri = %q, want /products/42 (query stripped)", rec.URI)
	}
	if rec.Millis != 150 {
		t.Errorf("ms = %v, want 150", rec.Millis)
	}
}

func TestIsColdStart(t *testing.T) {
	gap := 30 * time.Minute
	if IsColdStart(time.Time{}, false, base, gap) {
		t.Error("first request ever seen must not be a cold start")
	}
	if IsColdStart(base.Add(-time.Minute), true, base, gap) {
		t.Error("a request one minute after the last must not be cold")
	}
	if !IsColdStart(base.Add(-time.Hour), true, base, gap) {
		t.Error("a request an hour after the last must be cold")
	}
	if IsColdStart(base.Add(-time.Hour), true, base, 0) {
		t.Error("a zero gap disables cold detection")
	}
}

func TestStorePruneDropsOldRows(t *testing.T) {
	s := tempStore(t)
	seed(t, s, mk(4, -48*time.Hour, "app", "GET", "GET /old", "/old", 200, 20))
	seed(t, s, mk(4, 0, "app", "GET", "GET /new", "/new", 200, 20))

	n, err := s.Prune(base.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 4 {
		t.Errorf("pruned = %d, want 4", n)
	}
	a, _ := s.SiteAnalytics("app", base.Add(-72*time.Hour), base.Add(time.Hour))
	if a.Samples != 4 {
		t.Errorf("remaining samples = %d, want 4", a.Samples)
	}
}
