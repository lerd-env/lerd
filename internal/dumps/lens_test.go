package dumps

import (
	"strings"
	"testing"
)

func lensEvent(kind string, ctx Context) Event {
	return Event{V: 1, ID: "x", TS: "2026-05-10T12:00:00.000Z", Kind: kind, Ctx: ctx, Src: Source{File: "/x.php", Line: 1}}
}

func TestGroupKey(t *testing.T) {
	web := Context{Type: "fpm", Site: "acme", Request: "GET /checkout", PID: 7}
	parent := web
	worktree := web
	worktree.Branch = "feature-x"
	if groupKey(lensEvent(KindQuery, parent)) == groupKey(lensEvent(KindQuery, worktree)) {
		t.Error("a worktree request shares the parent's group")
	}
	withRID := worktree
	withRID.RID = "r1"
	if got := groupKey(lensEvent(KindQuery, withRID)); got != "rid:r1" {
		t.Errorf("rid group = %q, want rid:r1", got)
	}
	// dump() can run without the extension, whose rid then changes per call.
	a, b := web, web
	a.RID, b.RID = "vol-1", "vol-2"
	ka, kb := groupKey(lensEvent(KindDump, a)), groupKey(lensEvent(KindDump, b))
	if ka != kb || strings.HasPrefix(ka, "rid:") {
		t.Errorf("dump keys = %q, %q, want one request group", ka, kb)
	}
	cli := Context{Type: "cli", Site: "acme", PID: 7}
	cliBranch := cli
	cliBranch.Branch = "feature-x"
	if groupKey(lensEvent(KindQuery, cli)) == groupKey(lensEvent(KindQuery, cliBranch)) {
		t.Error("a worktree CLI run shares the parent's bucket")
	}
	later := lensEvent(KindQuery, cli)
	later.TS = "2026-05-10T12:00:06.000Z"
	if groupKey(lensEvent(KindQuery, cli)) == groupKey(later) {
		t.Error("CLI events 6s apart share a 5s bucket")
	}
}

func TestRouteOf(t *testing.T) {
	for _, c := range []struct {
		ctx  Context
		want string
	}{
		{Context{Type: "fpm", Request: "GET /users/5?tab=1"}, "GET /users/:id"},
		{Context{Type: "cli"}, ""},
		{Context{Type: "browser", Request: "https://acme.test/users/5?x=1"}, "GET /users/:id"},
	} {
		if got := routeOf(lensEvent(KindQuery, c.ctx)); got != c.want {
			t.Errorf("routeOf(%q) = %q, want %q", c.ctx.Request, got, c.want)
		}
	}
}

func TestNormalizeSQLCollapsesLiterals(t *testing.T) {
	if normalizeSQL("SELECT * FROM users WHERE id = 1") != normalizeSQL("select * from users where id = 42") {
		t.Error("numbers did not collapse")
	}
	if normalizeSQL("select * from t where name = 'bob'") != normalizeSQL("SELECT * FROM t WHERE name = 'alice'") {
		t.Error("strings did not collapse")
	}
}

func TestHaystackCarriesTheRequestIDAndRoute(t *testing.T) {
	ctx := Context{Type: "fpm", Site: "acme", Request: "GET /users/5", RID: "0065d3ba3290d4825"}
	q := lensEvent(KindQuery, ctx)
	q.Data = []byte(`{"sql":"select 1"}`)
	v := lensEvent(KindView, ctx)
	v.Data = []byte(`{"name":"Welcome"}`)
	d := lensEvent(KindDump, ctx)
	for _, e := range []Event{q, v, d} {
		hay := haystack(e)
		if !strings.Contains(hay, "0065d3ba3290d4825") || !strings.Contains(hay, "get /users/:id") {
			t.Errorf("%s haystack %q lacks the request id or route", e.Kind, hay)
		}
	}
	if !strings.Contains(haystack(v), "welcome") {
		t.Error("a view's haystack lacks its data")
	}
	if !strings.Contains(haystack(q), "select 1") {
		t.Error("a query's haystack lacks its SQL")
	}
}

func TestFacetOf(t *testing.T) {
	for _, c := range []struct {
		kind, data, want string
	}{
		{KindJob, `{"status":"failed"}`, "failed"},
		{KindLog, `{"level":"error"}`, "error"},
		{KindBrowser, `{"type":"console","level":"warn"}`, "console.warn"},
		{KindBrowser, `{"type":"error"}`, "error"},
		{KindView, `{"name":"x"}`, ""},
		{KindMessage, `{"channel":"sms","to":"+40"}`, "sms"},
	} {
		e := lensEvent(c.kind, Context{Type: "fpm"})
		e.Data = []byte(c.data)
		if got := facetOf(e); got != c.want {
			t.Errorf("facetOf(%s %s) = %q, want %q", c.kind, c.data, got, c.want)
		}
	}
}
