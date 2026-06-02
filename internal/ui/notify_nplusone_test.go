package ui

import (
	"encoding/json"
	"testing"

	"github.com/geodro/lerd/internal/dumps"
)

func qEvent(rid, request, sql string) dumps.Event {
	data, _ := json.Marshal(dumps.QueryData{SQL: sql, TimeMS: 1})
	return dumps.Event{
		V:    1,
		Kind: dumps.KindQuery,
		Ctx:  dumps.Context{Type: "fpm", Site: "acme", Request: request, RID: rid},
		Data: data,
	}
}

func TestNPlusOne_FiresOnceAtThresholdPerRoute(t *testing.T) {
	tr := newNPlusOneTracker()
	sql := "select * from posts where user_id = 1"

	// First two identical queries in the request: no warning yet.
	if n := tr.observe(qEvent("r1", "GET /feed", sql)); n != nil {
		t.Fatal("fired before threshold")
	}
	if n := tr.observe(qEvent("r1", "GET /feed", "select * from posts where user_id = 2")); n != nil {
		t.Fatal("fired at 2 (threshold is 3)")
	}
	// Third crosses the threshold -> one notification.
	n := tr.observe(qEvent("r1", "GET /feed", "select * from posts where user_id = 3"))
	if n == nil {
		t.Fatal("expected a notification at the threshold")
	}
	if n.Kind != "nplusone" {
		t.Errorf("kind = %q", n.Kind)
	}

	// A later request to the same route must NOT nag again.
	for i := 0; i < 5; i++ {
		if n := tr.observe(qEvent("r2", "GET /feed", sql)); n != nil {
			t.Fatal("nagged on a second request to the same route")
		}
	}
}

func TestNPlusOne_DistinctRoutesFireIndependently(t *testing.T) {
	tr := newNPlusOneTracker()
	sql := "select * from posts where user_id = 1"
	fire := func(rid, route string) bool {
		var got bool
		for i := 0; i < nPlusOneThreshold; i++ {
			if tr.observe(qEvent(rid, route, sql)) != nil {
				got = true
			}
		}
		return got
	}
	if !fire("a", "GET /feed") {
		t.Error("route /feed should fire")
	}
	if !fire("b", "GET /timeline") {
		t.Error("distinct route /timeline should fire independently")
	}
}

func TestNPlusOne_IdMaskedRoutesShareKey(t *testing.T) {
	tr := newNPlusOneTracker()
	sql := "select * from comments where post_id = 9"
	// /posts/1 trips the warning...
	for i := 0; i < nPlusOneThreshold; i++ {
		tr.observe(qEvent("r1", "GET /posts/1", sql))
	}
	// ...so /posts/2 (same route shape) must not nag.
	for i := 0; i < nPlusOneThreshold; i++ {
		if tr.observe(qEvent("r2", "GET /posts/2", sql)) != nil {
			t.Fatal("id-masked route should share the warned key")
		}
	}
}

func TestNPlusOne_DistinctQueriesDoNotTrip(t *testing.T) {
	tr := newNPlusOneTracker()
	for i, sql := range []string{"select * from a", "select * from b", "select * from c"} {
		if n := tr.observe(qEvent("r1", "GET /x", sql)); n != nil {
			t.Fatalf("distinct query %d should not trip N+1", i)
		}
	}
}

func TestNPlusOne_NoRidSkips(t *testing.T) {
	tr := newNPlusOneTracker()
	ev := qEvent("", "GET /x", "select 1")
	for i := 0; i < 5; i++ {
		if tr.observe(ev) != nil {
			t.Fatal("events without a request id must be ignored")
		}
	}
}
