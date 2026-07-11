package ui

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
	"github.com/geodro/lerd/internal/reqstats"
)

func aqRouteEvent(rid, request, sql string) dumps.Event {
	d, _ := json.Marshal(dumps.QueryData{SQL: sql, TimeMS: 2})
	return dumps.Event{
		Kind: dumps.KindQuery,
		Ctx:  dumps.Context{Type: "fpm", Site: "acme", Request: request, RID: rid},
		Src:  dumps.Source{File: "/app/User.php", Line: 30},
		Data: d,
	}
}

func TestJoinOptimize_AttachesEvidenceToRoute(t *testing.T) {
	stats := reqstats.SiteStats{
		Site:         "acme",
		MedianMillis: 40,
		Samples:      200,
		Slow: []reqstats.RouteStat{
			{Route: "GET /users/:id", Method: "GET", Example: "/users/5", P95Millis: 900, Multiplier: 22.5, Samples: 12},
			{Route: "GET /reports", Method: "GET", Example: "/reports", P95Millis: 500, Multiplier: 12, Samples: 8},
		},
	}
	// An N+1 on a concrete /users/5 hit: the join must normalize it to /users/:id
	// and land the finding on that flagged route.
	events := []dumps.Event{
		aqRouteEvent("r1", "GET /users/5", "select * from posts where user_id = 1"),
		aqRouteEvent("r1", "GET /users/5", "select * from posts where user_id = 2"),
		aqRouteEvent("r1", "GET /users/5", "select * from posts where user_id = 3"),
	}

	rep := joinOptimize(stats, events, 0, 0)
	if rep.Site != "acme" || rep.MedianMillis != 40 {
		t.Fatalf("report head = %+v", rep)
	}
	if len(rep.Routes) != 2 {
		t.Fatalf("routes = %d, want 2", len(rep.Routes))
	}
	users := rep.Routes[0]
	if users.Route != "GET /users/:id" {
		t.Fatalf("route[0] = %q", users.Route)
	}
	if len(users.Evidence) != 1 || len(users.Evidence[0].NPlusOne) != 1 {
		t.Fatalf("want one N+1 attached to /users/:id, got %+v", users.Evidence)
	}
	if got := users.Evidence[0].NPlusOne[0].Count; got != 3 {
		t.Errorf("n+1 count = %d, want 3", got)
	}
	// A slow route with no captured queries carries no evidence, and never nil-panics.
	if len(rep.Routes[1].Evidence) != 0 {
		t.Errorf("route[1] evidence = %+v, want none", rep.Routes[1].Evidence)
	}
}

func TestJoinOptimize_NoSlowRoutesIsEmpty(t *testing.T) {
	stats := reqstats.SiteStats{Site: "acme", MedianMillis: 40}
	rep := joinOptimize(stats, nil, 0, 0)
	if len(rep.Routes) != 0 {
		t.Fatalf("routes = %+v, want none", rep.Routes)
	}
	if rep.Routes == nil {
		t.Error("Routes should serialize as [], not null")
	}
}

func TestAttachProfiles_HangsHotspotsOnMatchingRoute(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{Name: "acme", Path: t.TempDir(), Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	// A captured SPX report for a concrete hit of the flagged route.
	dir := config.SpxDataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	trace := "[events]\n0 1 0 0\n1 1 100 0\n1 0 1100 0\n0 0 1200 0\n[functions]\nmain\nslow\n"
	os.WriteFile(filepath.Join(dir, "r.json"), []byte(`{"exec_ts":1000,"http_method":"GET","http_host":"acme.test","http_request_uri":"/users/7","cli":0}`), 0o644)
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte(trace))
	gw.Close()
	os.WriteFile(filepath.Join(dir, "r.txt.gz"), buf.Bytes(), 0o644)

	report := OptimizeReport{Routes: []RouteOptimization{
		{RouteStat: reqstats.RouteStat{Route: "GET /users/:id", Method: "GET"}},
		{RouteStat: reqstats.RouteStat{Route: "POST /nope", Method: "POST"}},
	}}
	attachProfiles(&report, "acme", "")

	if report.Routes[0].Profile == nil {
		t.Fatal("expected a profile attached to the captured route")
	}
	if h := report.Routes[0].Profile.Hotspots; len(h) == 0 || h[0].Function != "slow" {
		t.Errorf("hotspots = %+v, want slow on top", h)
	}
	if report.Routes[1].Profile != nil {
		t.Error("a route with no capture must have no profile")
	}

	// The same capture belongs to the parent's domain, so it must not be attached
	// to a worktree's report: a branch shows the hotspots of its own requests.
	wtReport := OptimizeReport{Routes: []RouteOptimization{
		{RouteStat: reqstats.RouteStat{Route: "GET /users/:id", Method: "GET"}},
	}}
	attachProfiles(&wtReport, "acme", "feature-x")
	if wtReport.Routes[0].Profile != nil {
		t.Error("the parent's capture leaked into the worktree's report")
	}
}
