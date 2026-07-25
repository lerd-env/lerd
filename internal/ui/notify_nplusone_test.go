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

// TestNPlusOne_BodyAlwaysNamesWhere pins that every notification points
// somewhere: a console run names its command, and an event with neither a
// request nor a command falls back to the line that fired the query rather
// than the generic "in one request".
func TestNPlusOne_BodyAlwaysNamesWhere(t *testing.T) {
	ev := func(ctx dumps.Context, src dumps.Source) dumps.Event {
		e := qEvent("r1", "", "select 1")
		e.Ctx, e.Src = ctx, src
		return e
	}
	cases := []struct {
		name string
		ev   dumps.Event
		want string
	}{
		{"worker wins", ev(dumps.Context{Worker: "queue:work", Command: "artisan queue:work", Request: "GET /x"}, dumps.Source{}),
			"queue:work ran a similar query 3×"},
		{"command over source", ev(dumps.Context{Command: "artisan sync:users --all"}, dumps.Source{File: "/var/www/app/Console/Sync.php", Line: 12}),
			"artisan sync:users --all ran a similar query 3×"},
		{"request when web", ev(dumps.Context{Request: "GET /orders"}, dumps.Source{File: "/var/www/app/X.php", Line: 3}),
			"GET /orders ran a similar query 3×"},
		{"source as last resort", ev(dumps.Context{}, dumps.Source{File: "/var/www/html/app/Jobs/SyncUsers.php", Line: 42}),
			"app/Jobs/SyncUsers.php:42 ran a similar query 3×"},
		{"nothing at all", ev(dumps.Context{}, dumps.Source{}),
			"Ran a similar query 3× in one request"},
	}
	for _, c := range cases {
		if got := notificationForNPlusOne(c.ev, nPlusOneThreshold).Body; got != c.want {
			t.Errorf("%s: body = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestNPlusOne_DistinctCommandsFireIndependently pins that the dedup key
// separates console invocations, so warning about one artisan command doesn't
// silence every other command for the rest of the session.
func TestNPlusOne_DistinctCommandsFireIndependently(t *testing.T) {
	tr := newNPlusOneTracker()
	fire := func(rid, command string) bool {
		var got bool
		for i := 0; i < nPlusOneThreshold; i++ {
			e := qEvent(rid, "", "select * from users where id = 1")
			e.Ctx = dumps.Context{Type: "cli", Site: "acme", RID: rid, Command: command}
			if tr.observe(e) != nil {
				got = true
			}
		}
		return got
	}
	if !fire("a", "artisan sync:users") {
		t.Error("first command should fire")
	}
	if !fire("b", "artisan import:orders") {
		t.Error("a different command should fire independently")
	}
	if fire("c", "artisan sync:users") {
		t.Error("the same command must not nag twice")
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
