package dumps

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func queryEvent(id, rid, sql string, ms float64) Event {
	e := Event{V: 1, ID: id, TS: "2026-05-10T12:00:00.000Z", Kind: KindQuery,
		Ctx: Context{Type: "fpm", Site: "acme", Request: "GET /orders", RID: rid}}
	e.Data, _ = json.Marshal(QueryData{SQL: sql, TimeMS: ms})
	return e
}

func groupIDs(g Group) []string {
	out := make([]string, len(g.Rows))
	for i, r := range g.Rows {
		var e Event
		_ = json.Unmarshal(r.Event, &e)
		out[i] = e.ID
	}
	return out
}

func TestRing_GroupsByRequestNewestFirst(t *testing.T) {
	r := NewRing(50)
	r.Append(queryEvent("a", "r1", "select 1", 1))
	r.Append(queryEvent("b", "r2", "select 2", 1))
	r.Append(queryEvent("c", "r1", "select 3", 1))
	page := r.Groups(GroupOpts{FilterOpts: FilterOpts{Kind: KindQuery}, Limit: 10, Rows: 10})
	if len(page.Groups) != 2 || page.Groups[0].Key != "rid:r1" || page.Groups[1].Key != "rid:r2" {
		t.Fatalf("groups = %+v, want r1 (latest activity) then r2", page.Groups)
	}
	if fmt.Sprint(groupIDs(page.Groups[0])) != "[c a]" || page.Groups[0].Count != 2 {
		t.Errorf("r1 rows = %v count %d, want newest first, 2", groupIDs(page.Groups[0]), page.Groups[0].Count)
	}
}

func TestRing_GroupsPageBackByCursor(t *testing.T) {
	r := NewRing(50)
	for i := 1; i <= 5; i++ {
		r.Append(queryEvent(fmt.Sprintf("e%d", i), fmt.Sprintf("r%d", i), "select 1", 1))
	}
	first := r.Groups(GroupOpts{FilterOpts: FilterOpts{Kind: KindQuery}, Limit: 2, Rows: 10})
	if len(first.Groups) != 2 || first.Groups[0].Key != "rid:r5" || first.Next == 0 || first.Total != 5 {
		t.Fatalf("first page = %+v", first)
	}
	next := r.Groups(GroupOpts{FilterOpts: FilterOpts{Kind: KindQuery}, Limit: 2, Rows: 10, Before: first.Next})
	if len(next.Groups) != 2 || next.Groups[0].Key != "rid:r3" {
		t.Errorf("second page = %+v, want r3 and r2", next.Groups)
	}
}

func TestRing_GroupsCapRowsAndPageWithinAGroup(t *testing.T) {
	r := NewRing(50)
	for i := 1; i <= 5; i++ {
		r.Append(queryEvent(fmt.Sprintf("e%d", i), "r1", "select 1", 1))
	}
	opts := GroupOpts{FilterOpts: FilterOpts{Kind: KindQuery}, Limit: 10, Rows: 2}
	g := r.Groups(opts).Groups[0]
	if g.Count != 5 || fmt.Sprint(groupIDs(g)) != "[e5 e4]" {
		t.Fatalf("group = count %d rows %v, want 5 and the newest two", g.Count, groupIDs(g))
	}
	more := r.GroupRows(opts, g.Key, 2)
	if fmt.Sprint(groupIDs(Group{Rows: more})) != "[e3 e2]" {
		t.Errorf("more rows = %v", groupIDs(Group{Rows: more}))
	}
}

func TestRing_GroupsFlagDuplicatesAndNPlusOneAndSumTime(t *testing.T) {
	r := NewRing(50)
	for i := 1; i <= 3; i++ {
		r.Append(queryEvent(fmt.Sprintf("n%d", i), "r1", fmt.Sprintf("select * from users where id = %d", i), 40))
	}
	r.Append(queryEvent("s", "r1", "select * from orders", 150))
	r.Append(queryEvent("o", "r2", "select * from users where id = 9", 1))
	page := r.Groups(GroupOpts{FilterOpts: FilterOpts{Kind: KindQuery}, Limit: 10, Rows: 10})
	var g1, g2 Group
	for _, g := range page.Groups {
		if g.Key == "rid:r1" {
			g1 = g
		} else {
			g2 = g
		}
	}
	if !g1.NPlusOne || g1.TotalMS != 270 || g1.SlowCount != 1 {
		t.Errorf("r1 = n+1 %v total %v slow %d, want true 270 1", g1.NPlusOne, g1.TotalMS, g1.SlowCount)
	}
	for _, row := range g1.Rows {
		var e Event
		_ = json.Unmarshal(row.Event, &e)
		if want := map[bool]int{true: 1, false: 3}[e.ID == "s"]; row.Dup != want {
			t.Errorf("row %s dup = %d, want %d", e.ID, row.Dup, want)
		}
	}
	if g2.NPlusOne || g2.Rows[0].Dup != 1 {
		t.Errorf("r2 counted another request's queries: %+v", g2)
	}
}

func TestRing_GroupsSearchWorkerFacetAndRoute(t *testing.T) {
	r := NewRing(50)
	r.Append(queryEvent("a", "r1", "select * from invoices", 1))
	w := queryEvent("b", "r2", "select * from jobs", 1)
	w.Ctx.Type, w.Ctx.Request, w.Ctx.Worker = "cli", "", "queue:work"
	r.Append(w)
	job := Event{V: 1, ID: "j", TS: "2026-05-10T12:00:00.000Z", Kind: KindJob,
		Ctx: Context{Type: "cli", Site: "acme", Worker: "queue:work", RID: "r3"}, Data: []byte(`{"status":"failed"}`)}
	r.Append(job)
	keys := func(o GroupOpts) string {
		o.Limit, o.Rows = 10, 10
		var out []string
		for _, g := range r.Groups(o).Groups {
			out = append(out, g.Key)
		}
		return fmt.Sprint(out)
	}
	if got := keys(GroupOpts{FilterOpts: FilterOpts{Kind: KindQuery}, Search: "INVOICES"}); got != "[rid:r1]" {
		t.Errorf("search = %s", got)
	}
	if got := keys(GroupOpts{FilterOpts: FilterOpts{Kind: KindQuery}, HideWorkers: true}); got != "[rid:r1]" {
		t.Errorf("workers hidden = %s", got)
	}
	if got := keys(GroupOpts{FilterOpts: FilterOpts{Kind: KindJob}, HideWorkers: true}); got != "[rid:r3]" {
		t.Errorf("a worker's jobs show whatever the worker toggle says, got %s", got)
	}
	if got := keys(GroupOpts{FilterOpts: FilterOpts{Kind: KindQuery}, Worker: "queue:work"}); got != "[rid:r2]" {
		t.Errorf("worker = %s", got)
	}
	if got := keys(GroupOpts{FilterOpts: FilterOpts{Kind: KindJob}, Facet: "done"}); got != "[]" {
		t.Errorf("facet done = %s", got)
	}
	if got := keys(GroupOpts{FilterOpts: FilterOpts{Kind: KindQuery, Route: "GET /orders"}}); got != "[rid:r1]" {
		t.Errorf("route = %s", got)
	}
}

func TestRing_WorkersListsWorkerCommands(t *testing.T) {
	r := NewRing(10)
	w := queryEvent("a", "r1", "select 1", 1)
	w.Ctx.Worker = "queue:work"
	r.Append(w)
	r.Append(queryEvent("b", "r2", "select 1", 1))
	if got := r.Workers(FilterOpts{Site: "acme"}); fmt.Sprint(got) != "[queue:work]" {
		t.Errorf("workers = %v", got)
	}
}

// A worker's rows for a Laravel job carry no payload, which is only readable
// where the job was dispatched, so they take it from the queued row.
func TestRing_JobRowBorrowsThePayloadOfItsQueuedRow(t *testing.T) {
	r := NewRing(50)
	queued := Event{V: 1, ID: "q", TS: "2026-05-10T12:00:00.000Z", Kind: KindJob,
		Ctx: Context{Type: "fpm", Site: "acme", RID: "r1"}, Data: []byte(`{"uuid":"u1","status":"queued","payload":{"order":"7"}}`)}
	done := Event{V: 1, ID: "d", TS: "2026-05-10T12:00:01.000Z", Kind: KindJob,
		Ctx: Context{Type: "cli", Site: "acme", RID: "r2", Worker: "queue:work"}, Data: []byte(`{"uuid":"u1","status":"processed"}`)}
	r.Append(queued)
	r.Append(done)
	page := r.Groups(GroupOpts{FilterOpts: FilterOpts{Kind: KindJob}, Limit: 10, Rows: 10})
	var got Event
	_ = json.Unmarshal(page.Groups[0].Rows[0].Event, &got)
	if got.ID != "d" || !strings.Contains(string(got.Data), `"order":"7"`) {
		t.Errorf("processed row = %s %s, want the queued row's payload", got.ID, got.Data)
	}
}

func TestRing_FacetValuesForAKind(t *testing.T) {
	r := NewRing(50)
	for i, st := range []string{"queued", "failed", "queued"} {
		r.Append(Event{V: 1, ID: fmt.Sprint(i), TS: "2026-05-10T12:00:00.000Z", Kind: KindJob,
			Ctx: Context{Type: "cli", Site: "acme"}, Data: []byte(`{"status":"` + st + `"}`)})
	}
	if got := r.FacetValues(FilterOpts{Site: "acme", Kind: KindJob}); fmt.Sprint(got) != "[failed queued]" {
		t.Errorf("facet values = %v", got)
	}
}

func TestRing_GroupsDoNotFlagDistinctQueriesAsNPlusOne(t *testing.T) {
	r := NewRing(50)
	for i, sql := range []string{"select * from users", "select * from orders", "select * from carts"} {
		r.Append(queryEvent(fmt.Sprint(i), "r1", sql, 1))
	}
	g := r.Groups(GroupOpts{FilterOpts: FilterOpts{Kind: KindQuery}, Limit: 10, Rows: 10}).Groups[0]
	if g.NPlusOne {
		t.Error("three different queries flagged as an N+1")
	}
}

func TestRing_GroupsSearchTheQuerySourceFile(t *testing.T) {
	r := NewRing(50)
	e := queryEvent("a", "r1", "select 1", 1)
	e.Src.File = "/app/Http/Controllers/OrderController.php"
	r.Append(e)
	r.Append(queryEvent("b", "r2", "select 1", 1))
	page := r.Groups(GroupOpts{FilterOpts: FilterOpts{Kind: KindQuery}, Search: "ordercontroller", Limit: 10, Rows: 10})
	if len(page.Groups) != 1 || page.Groups[0].Key != "rid:r1" {
		t.Errorf("search by file = %+v", page.Groups)
	}
}

// A lens lists rows without what only an expanded row shows, the call stack
// and a mail's HTML, and reads the whole event when one is opened.
func TestRing_GroupRowsLeaveOutTheDetailEventReturnsIt(t *testing.T) {
	r := NewRing(50)
	e := queryEvent("a", "r1", "select 1", 1)
	e.Data = []byte(`{"sql":"select 1","time_ms":1,"trace":[{"file":"/app/x.php","line":3,"func":"run"}]}`)
	r.Append(e)
	row := r.Groups(GroupOpts{FilterOpts: FilterOpts{Kind: KindQuery}, Limit: 10, Rows: 10}).Groups[0].Rows[0]
	if strings.Contains(string(row.Event), "trace") || !strings.Contains(string(row.Event), "select 1") {
		t.Errorf("listed row = %s, want the SQL without the trace", row.Event)
	}
	full, ok := r.Event("a")
	if !ok || !strings.Contains(string(full), `"trace"`) {
		t.Errorf("event = %s %v, want the whole event", full, ok)
	}
	if _, ok := r.Event("missing"); ok {
		t.Error("a missing event was found")
	}
}

func browserEvent(id, page, data string) Event {
	return Event{V: 1, ID: id, TS: "2026-05-10T12:00:00.000Z", Kind: KindBrowser,
		Ctx: Context{Type: "browser", Site: "acme", Request: "https://acme.test/", RID: page}, Data: []byte(data)}
}

func TestRing_GroupsLeavePageViewsOutOfTheRows(t *testing.T) {
	r := NewRing(50)
	r.Append(browserEvent("nav", "p1", `{"type":"navigation"}`))
	r.Append(browserEvent("err", "p1", `{"type":"error","message":"boom"}`))
	g := r.Groups(GroupOpts{FilterOpts: FilterOpts{Kind: KindBrowser}, Limit: 10, Rows: 10}).Groups[0]
	if fmt.Sprint(groupIDs(g)) != "[err]" || g.Count != 1 {
		t.Errorf("rows = %v count %d, want only what happened on the page", groupIDs(g), g.Count)
	}
}

func TestRing_OneRequestHoldsTheBrowserFailuresThatReachedIt(t *testing.T) {
	r := NewRing(50)
	r.Append(queryEvent("q", "r1", "select 1", 1))
	r.Append(browserEvent("fetch", "page9", `{"type":"network","status":500,"rid":"r1"}`))
	r.Append(browserEvent("other", "page9", `{"type":"network","status":500,"rid":"r2"}`))
	page := r.Groups(GroupOpts{FilterOpts: FilterOpts{Kind: KindBrowser, RID: "r1"}, Limit: 10, Rows: 10})
	if len(page.Groups) != 1 || fmt.Sprint(groupIDs(page.Groups[0])) != "[fetch]" {
		t.Errorf("browser rows for r1 = %+v", page.Groups)
	}
	if got := r.Counts(FilterOpts{RID: "r1"}); got[KindQuery] != 1 || got[KindBrowser] != 1 {
		t.Errorf("counts for r1 = %v", got)
	}
}

func TestRing_DumpGroupsFilterByContextAndSearchTheirOutput(t *testing.T) {
	r := NewRing(50)
	web := Event{V: 1, ID: "w", TS: "2026-05-10T12:00:00.000Z", Kind: KindDump, Label: "cart",
		Ctx: Context{Type: "fpm", Site: "acme", Request: "GET /cart", PID: 1}, Src: Source{File: "/app/CartController.php"}, Text: "array:2"}
	cli := Event{V: 1, ID: "c", TS: "2026-05-10T12:00:00.000Z", Kind: KindDump, Label: "import",
		Ctx: Context{Type: "cli", Site: "acme", PID: 2}, Text: "done"}
	r.Append(web)
	r.Append(cli)
	keys := func(o GroupOpts) int { o.Kind, o.Limit, o.Rows = KindDump, 10, 10; return len(r.Groups(o).Groups) }
	if keys(GroupOpts{FilterOpts: FilterOpts{Ctx: "cli"}}) != 1 {
		t.Error("ctx cli did not narrow to the console dump")
	}
	for _, q := range []string{"cart", "array:2", "cartcontroller"} {
		if keys(GroupOpts{Search: q}) != 1 {
			t.Errorf("search %q did not find the web dump by label, output or file", q)
		}
	}
}
