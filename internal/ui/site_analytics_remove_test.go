package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/reqstats"
)

// seedAnalytics fills the durable store the analytics endpoints read, using the
// same path the handler resolves so the two meet on the test's XDG dirs.
func seedAnalytics(t *testing.T, recs []reqstats.Record) *reqstats.Store {
	t.Helper()
	store, err := reqstats.OpenShared(config.RequestStatsDB())
	if err != nil {
		t.Fatalf("OpenShared: %v", err)
	}
	if err := store.Insert(recs); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	return store
}

func analyticsRecord(site, route, uri string, at time.Time) reqstats.Record {
	return reqstats.Record{
		At: at, Site: site, Route: route, Method: "GET", Status: 200, Millis: 40, URI: uri,
	}
}

func getAnalytics(t *testing.T, domain string) analyticsResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+domain+"/analytics?range=24h", nil)
	rec := httptest.NewRecorder()
	if !analyticsRoute(rec, req, domain, []string{"analytics"}) {
		t.Fatal("analyticsRoute did not handle the read")
	}
	var got analyticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

func postRemove(t *testing.T, domain, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+domain+"/analytics/remove", strings.NewReader(body))
	rec := httptest.NewRecorder()
	if !analyticsRoute(rec, req, domain, []string{"analytics", "remove"}) {
		t.Fatal("analyticsRoute did not handle the removal")
	}
	return rec
}

func TestAnalyticsRemoveRouteDropsItsHistory(t *testing.T) {
	registerSite(t, "acme", "acme.test")
	now := time.Now()
	seedAnalytics(t, []reqstats.Record{
		analyticsRecord("acme", "GET /noisy", "/noisy", now.Add(-time.Minute)),
		analyticsRecord("acme", "GET /noisy", "/noisy", now.Add(-2*time.Minute)),
		analyticsRecord("acme", "GET /keep", "/keep", now.Add(-time.Minute)),
	})

	rec := postRemove(t, "acme.test", `{"route":"GET /noisy"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	got := getAnalytics(t, "acme.test")
	if got.Samples != 1 {
		t.Errorf("samples = %d, want 1", got.Samples)
	}
	if len(got.Excluded) != 0 {
		t.Errorf("a plain removal must not exclude anything, got %v", got.Excluded)
	}
}

func TestAnalyticsRemoveRequestDropsOnlyThatRow(t *testing.T) {
	registerSite(t, "acme", "acme.test")
	now := time.Now().Truncate(time.Millisecond)
	target := now.Add(-time.Minute)
	seedAnalytics(t, []reqstats.Record{
		analyticsRecord("acme", "GET /page", "/page", target),
		analyticsRecord("acme", "GET /page", "/page", now.Add(-2*time.Minute)),
	})

	body := `{"route":"GET /page","uri":"/page","at_millis":` +
		strconv.FormatInt(target.UnixMilli(), 10) + `}`
	if rec := postRemove(t, "acme.test", body); rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	got := getAnalytics(t, "acme.test")
	if got.Samples != 1 {
		t.Errorf("samples = %d, want 1", got.Samples)
	}
}

func TestAnalyticsRemoveWithExcludeHidesFutureTraffic(t *testing.T) {
	registerSite(t, "acme", "acme.test")
	now := time.Now()
	store := seedAnalytics(t, []reqstats.Record{
		analyticsRecord("acme", "GET /health", "/health", now.Add(-time.Minute)),
	})

	if rec := postRemove(t, "acme.test", `{"route":"GET /health","exclude":true}`); rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	got := getAnalytics(t, "acme.test")
	if len(got.Excluded) != 1 || got.Excluded[0] != "GET /health" {
		t.Fatalf("excluded = %v, want GET /health", got.Excluded)
	}

	// Traffic recorded before the watcher catches up must still stay out of sight.
	if err := store.Insert([]reqstats.Record{analyticsRecord("acme", "GET /health", "/health", now)}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got = getAnalytics(t, "acme.test")
	if got.Samples != 0 {
		t.Errorf("samples = %d, want 0 for an excluded route", got.Samples)
	}
	if len(got.Recent) != 0 {
		t.Errorf("recent = %v, want none for an excluded route", got.Recent)
	}
}

func TestAnalyticsUnexcludeRoute(t *testing.T) {
	registerSite(t, "acme", "acme.test")
	seedAnalytics(t, nil)
	if rec := postRemove(t, "acme.test", `{"route":"GET /health","exclude":true}`); rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/sites/acme.test/analytics/excludes?route=GET+%2Fhealth", nil)
	rec := httptest.NewRecorder()
	if !analyticsRoute(rec, req, "acme.test", []string{"analytics", "excludes"}) {
		t.Fatal("analyticsRoute did not handle the unexclude")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if got := getAnalytics(t, "acme.test"); len(got.Excluded) != 0 {
		t.Errorf("excluded = %v after unexclude, want none", got.Excluded)
	}
}

func TestAnalyticsRemoveRejectsMissingRoute(t *testing.T) {
	registerSite(t, "acme", "acme.test")
	seedAnalytics(t, nil)
	if rec := postRemove(t, "acme.test", `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
