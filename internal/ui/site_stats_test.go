package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/reqstats"
)

func TestStatsRoute_returnsSlowRoutes(t *testing.T) {
	registerSite(t, "acme", "acme.test")

	a := reqstats.New(func(h string) (string, bool) {
		if h == "acme.test" {
			return "acme", true
		}
		return "", false
	})
	for i := 0; i < 20; i++ {
		a.Record(reqstats.AccessRecord{Host: "acme.test", Status: 200, RequestTime: 0.04, Method: "GET", URI: "/home"})
	}
	for i := 0; i < 10; i++ {
		a.Record(reqstats.AccessRecord{Host: "acme.test", Status: 200, RequestTime: 0.38, Method: "GET", URI: "/reports/7"})
	}
	if err := a.Save(config.RequestStatsFile()); err != nil {
		t.Fatalf("save: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/acme.test/stats", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	if !statsRoute(rec, req, "acme.test", []string{"stats"}) {
		t.Fatal("statsRoute did not handle the request")
	}

	var got reqstats.SiteStats
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Slow) == 0 || got.Slow[0].Route != "GET /reports/:id" {
		t.Errorf("expected the slow reports route, got %+v", got.Slow)
	}
}

func TestStatsRoute_emptyWhenNoTraffic(t *testing.T) {
	registerSite(t, "acme", "acme.test")
	req := httptest.NewRequest(http.MethodGet, "/api/sites/acme.test/stats", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	if !statsRoute(rec, req, "acme.test", []string{"stats"}) {
		t.Fatal("statsRoute did not handle the request")
	}
	var got reqstats.SiteStats
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Slow) != 0 {
		t.Errorf("expected no slow routes, got %+v", got.Slow)
	}
}
