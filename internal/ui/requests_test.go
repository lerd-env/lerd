package ui

import (
	"encoding/json"
	"testing"

	"github.com/geodro/lerd/internal/dumps"
)

func ev(id, ts, kind, rid, site, typ string, data any) dumps.Event {
	raw, _ := json.Marshal(data)
	return dumps.Event{V: 1, ID: id, TS: ts, Kind: kind, Ctx: dumps.Context{Type: typ, Site: site, RID: rid, Request: "GET /cart"}, Data: raw}
}

// A page on one site calls an API on another; the API request links back to
// the page view that sent it, and the page lists it as a request it sent.
func TestRequests_LinkAPageToTheRequestsItSentAcrossSites(t *testing.T) {
	events := []dumps.Event{
		ev("1", "2026-10-04T10:00:00.900Z", dumps.KindBrowser, "page1", "shop", "browser", map[string]any{"type": "navigation", "message": "https://shop.test/cart", "url": "https://shop.test/cart"}),
		ev("2", "2026-10-04T10:00:00.950Z", dumps.KindBrowser, "page1", "shop", "browser", map[string]any{"type": "request", "message": "422 POST https://api.test/cart", "method": "POST", "request": "https://api.test/cart", "status": 422, "rid": "api9", "via": "fetch", "cross": true, "duration_ms": 45, "timing": map[string]any{"requestStart": 1, "responseStart": 40}}),
		ev("3", "2026-10-04T10:00:00.980Z", dumps.KindQuery, "api9", "api", "fpm", map[string]any{"sql": "select * from carts where id = ?", "bindings": []any{3}, "time_ms": 1.2}),
		ev("4", "2026-10-04T10:00:00.990Z", dumps.KindException, "api9", "api", "fpm", map[string]any{"type": "ValidationException", "message": "quantity"}),
		ev("5", "2026-10-04T10:00:01.000Z", dumps.KindRequest, "api9", "api", "fpm", map[string]any{"method": "POST", "uri": "/cart", "status": 422, "time_ms": 38, "route": "cart.store", "nginx_ms": 0.4, "queue_ms": 1.5}),
	}
	list := listRequests(events)
	if len(list) != 2 || list[0].RID != "api9" {
		t.Fatalf("list = %+v", list)
	}
	api := list[0]
	if api.Type != "fetch" || api.Status != 422 || api.Route != "cart.store" || api.NginxMS != 0.4 || api.QueueMS != 1.5 || api.Parent.Timing["responseStart"] != 40 || api.Parent == nil || api.Parent.RID != "page1" || api.Parent.Site != "shop" || !api.Parent.Cross {
		t.Fatalf("api = %+v parent %+v", api, api.Parent)
	}
	if want := []string{"4xx", "exception"}; len(api.Problems) != 2 || api.Problems[0] != want[0] || api.Problems[1] != want[1] {
		t.Fatalf("problems = %v", api.Problems)
	}
	page := list[1]
	if page.Type != "page" || len(page.Children) != 1 || page.Children[0].RID != "api9" || page.Children[0].Status != 422 {
		t.Fatalf("page = %+v", page)
	}

	if api.Started != "2026-10-04T10:00:00.962Z" {
		t.Fatalf("started = %q, want the end less the 38 ms it took", api.Started)
	}
	d, ok := requestDetail(events, "api9")
	if !ok || len(d.Events[dumps.KindQuery]) != 1 || d.Queries == nil || d.Queries.QueryCount != 1 || len(d.Events[dumps.KindRequest]) != 1 {
		t.Fatalf("detail = %+v", d)
	}
	if _, ok := requestDetail(events, "nope"); ok {
		t.Fatal("unknown request found")
	}
}

// A job run as its own process is a request of type job, and a sync job is
// listed under the request it ran in.
func TestRequests_JobsAreProcessesOfTheirOwn(t *testing.T) {
	events := []dumps.Event{
		ev("1", "2026-10-04T10:00:00.100Z", dumps.KindRequest, "web1", "shop", "fpm", map[string]any{"method": "POST", "uri": "/orders", "status": 201, "time_ms": 50}),
		ev("2", "2026-10-04T10:00:00.060Z", dumps.KindJob, "job1", "shop", "fpm", map[string]any{"status": "processing", "class": "App\\Jobs\\SendInvoice", "connection": "sync", "parent": "web1"}),
		ev("3", "2026-10-04T10:00:00.070Z", dumps.KindJob, "job1", "shop", "fpm", map[string]any{"status": "failed", "class": "App\\Jobs\\SendInvoice", "time_ms": 10}),
	}
	list := listRequests(events)
	var job, web RequestSummary
	for _, r := range list {
		if r.RID == "job1" {
			job = r
		} else {
			web = r
		}
	}
	if job.Type != "job" || job.Job != "App\\Jobs\\SendInvoice" || job.JobStatus != "failed" || job.TimeMS != 10 || job.Parent == nil || job.Parent.RID != "web1" {
		t.Fatalf("job = %+v parent %+v", job, job.Parent)
	}
	if len(job.Problems) != 1 || job.Problems[0] != "job failed" {
		t.Fatalf("job problems = %v", job.Problems)
	}
	if len(web.Children) != 1 || web.Children[0].RID != "job1" || web.Children[0].Via != "job" {
		t.Fatalf("web children = %+v", web.Children)
	}
}
