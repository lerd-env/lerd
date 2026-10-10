package ui

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/dumps"
)

func lensQuery(id, site, rid, sql string) dumps.Event {
	e := dumps.Event{V: 1, ID: id, TS: "2026-05-10T12:00:00.000Z", Kind: dumps.KindQuery,
		Ctx: dumps.Context{Type: "fpm", Site: site, Request: "GET /orders", RID: rid}}
	e.Data, _ = json.Marshal(dumps.QueryData{SQL: sql, TimeMS: 2})
	return e
}

func lensGet(t *testing.T, handler func(w *httptest.ResponseRecorder), out any) {
	t.Helper()
	rec := httptest.NewRecorder()
	handler(rec)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
}

func TestHandleDumpsCounts_CountsPerKindForTheSite(t *testing.T) {
	srv := withDumpsServer(t)
	// Test runs are recorded only while their capture is on.
	srv.SetKeepTests(true)
	srv.Push(lensQuery("a", "acme", "r1", "select 1"))
	srv.Push(lensQuery("b", "acme", "r1", "select 2"))
	test := lensQuery("c", "acme", "r2", "select 3")
	test.Ctx.Test = true
	srv.Push(test)
	srv.Push(lensQuery("d", "other", "r3", "select 4"))

	var got struct {
		Counts      map[string]int `json:"counts"`
		HiddenTests int            `json:"hidden_tests"`
	}
	lensGet(t, func(w *httptest.ResponseRecorder) {
		handleDumpsCounts(w, httptest.NewRequest("GET", "/api/dumps/counts?site=acme", nil))
	}, &got)
	if got.Counts["query"] != 2 || got.HiddenTests != 1 {
		t.Errorf("counts = %+v", got)
	}
}

func TestHandleDumpsGroups_PagesTheLens(t *testing.T) {
	srv := withDumpsServer(t)
	srv.Push(lensQuery("a", "acme", "r1", "select * from invoices"))
	srv.Push(lensQuery("b", "acme", "r2", "select * from users"))
	srv.Push(lensQuery("c", "acme", "r3", "select * from users"))

	var page dumps.GroupPage
	lensGet(t, func(w *httptest.ResponseRecorder) {
		handleDumpsGroups(w, httptest.NewRequest("GET", "/api/dumps/groups?site=acme&kind=query&limit=1", nil))
	}, &page)
	if len(page.Groups) != 1 || page.Groups[0].Key != "rid:r3" || page.Next == 0 {
		t.Fatalf("first page = %+v", page)
	}
	var searched dumps.GroupPage
	lensGet(t, func(w *httptest.ResponseRecorder) {
		handleDumpsGroups(w, httptest.NewRequest("GET", "/api/dumps/groups?site=acme&kind=query&q=invoices", nil))
	}, &searched)
	if len(searched.Groups) != 1 || searched.Groups[0].Key != "rid:r1" {
		t.Errorf("search = %+v", searched)
	}
}

func TestHandleDumpsGroupRows_ReadsFurtherIntoAGroup(t *testing.T) {
	srv := withDumpsServer(t)
	for _, id := range []string{"a", "b", "c"} {
		srv.Push(lensQuery(id, "acme", "r1", "select 1"))
	}
	var rows []dumps.Row
	lensGet(t, func(w *httptest.ResponseRecorder) {
		handleDumpsGroupRows(w, httptest.NewRequest("GET", "/api/dumps/groups/rows?site=acme&kind=query&key=rid:r1&offset=2", nil))
	}, &rows)
	if len(rows) != 1 || !strings.Contains(string(rows[0].Event), `"id":"a"`) {
		t.Errorf("rows = %+v", rows)
	}
}

func TestHandleDumpsFacets_ListsSitesAndWorkers(t *testing.T) {
	srv := withDumpsServer(t)
	w := lensQuery("a", "acme", "r1", "select 1")
	w.Ctx.Worker = "queue:work"
	srv.Push(w)
	srv.Push(lensQuery("b", "beta", "r2", "select 1"))
	var got struct {
		Sites   []string `json:"sites"`
		Workers []string `json:"workers"`
	}
	lensGet(t, func(rec *httptest.ResponseRecorder) {
		handleDumpsFacets(rec, httptest.NewRequest("GET", "/api/dumps/facets?kind=query", nil))
	}, &got)
	if strings.Join(got.Sites, ",") != "acme,beta" || strings.Join(got.Workers, ",") != "queue:work" {
		t.Errorf("facets = %+v", got)
	}
}

// A lens refetches what it shows; the stream only tells it something arrived,
// so a tab never receives events it does not render.
func TestHandleDumpsStream_NotifyModeSendsOnlyTheNudge(t *testing.T) {
	srv := withDumpsServer(t)
	srv.Push(lensQuery("old", "acme", "r0", "select 1"))
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/dumps/stream?notify=1&site=acme", nil).WithContext(ctx)
	rec := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleDumpsStream(rec, req)
	}()
	time.Sleep(50 * time.Millisecond)
	srv.Push(lensQuery("live", "acme", "r1", "select secret_column"))
	srv.Push(lensQuery("elsewhere", "other", "r2", "select 1"))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(rec.bodyString(), `"kind":"query"`) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	body := rec.bodyString()
	if !strings.Contains(body, `"kind":"query"`) || strings.Contains(body, "secret_column") || strings.Contains(body, "old") {
		t.Errorf("notify stream body:\n%s", body)
	}
	if strings.Count(body, "data:") != 1 {
		t.Errorf("want one nudge for the site, got:\n%s", body)
	}
}

func TestHandleDumpsEvent_ReturnsTheWholeEventOrNotFound(t *testing.T) {
	srv := withDumpsServer(t)
	e := lensQuery("a", "acme", "r1", "select 1")
	e.Data = []byte(`{"sql":"select 1","time_ms":1,"trace":[{"file":"/x.php","line":1,"func":"f"}]}`)
	srv.Push(e)
	rec := httptest.NewRecorder()
	handleDumpsEvent(rec, httptest.NewRequest("GET", "/api/dumps/event?id=a", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"trace"`) {
		t.Errorf("event = %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	handleDumpsEvent(rec, httptest.NewRequest("GET", "/api/dumps/event?id=nope", nil))
	if rec.Code != 404 {
		t.Errorf("missing event = %d, want 404", rec.Code)
	}
}
