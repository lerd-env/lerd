package ui

import (
	"encoding/json"
	"testing"

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
