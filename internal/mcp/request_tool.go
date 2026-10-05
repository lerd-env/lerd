package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

func requestTool() mcpTool {
	return mcpTool{
		Name:        "request",
		Description: "A recorded request, all the Debug window shows, by its X-Lerd-Rid. action: list, tabs (the app's own too), summary (timing, findings, exceptions, GraphQL, sent requests), tab (rows, paged), trace (by trace_ref).",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"action": {Type: "string", Enum: []string{"list", "tabs", "summary", "tab", "trace"}},
				"rid":    {Type: "string", Description: "Request id."},
				"tab":    {Type: "string", Description: "tab: id from tabs."},
				"offset": {Type: "integer", Description: "tab: rows to skip."},
				"limit":  {Type: "integer", Description: "list/tab: rows (tab default 50)."},
				"ref":    {Type: "integer", Description: "trace: trace_ref."},
				"site":   {Type: "string", Description: "list: site filter."},
				"branch": {Type: "string", Description: "list: branch filter."},
			},
			Required: []string{"action"},
		},
	}
}

// reqDetail is the request detail lerd-ui serves at /api/requests/{rid},
// read loosely: the tool reshapes it per tab rather than relying on its types.
type reqDetail struct {
	Head    map[string]any
	Events  map[string][]reqEvent `json:"events"`
	Queries map[string]any        `json:"queries"`
	Traces  []json.RawMessage     `json:"traces"`
	started time.Time
}

type reqEvent struct {
	ID    string         `json:"id"`
	TS    string         `json:"ts"`
	Label string         `json:"label"`
	Text  string         `json:"text"`
	Src   map[string]any `json:"src"`
	Data  map[string]any `json:"data"`
}

func (d *reqDetail) ev(kind string) []reqEvent { return d.Events[kind] }

func (d *reqDetail) request() map[string]any {
	if evs := d.ev("request"); len(evs) > 0 && evs[0].Data != nil {
		return evs[0].Data
	}
	return map[string]any{}
}

// row is an event as one line of a tab: its place, when it happened on the
// request's clock, where it came from and what it said.
func (d *reqDetail) row(n int, e reqEvent) map[string]any {
	r := map[string]any{"n": n}
	for k, v := range e.Data {
		if k != "trace" {
			r[k] = v
		}
	}
	if e.Label != "" {
		r["label"] = e.Label
	}
	if e.Text != "" {
		r["text"] = e.Text
	}
	if f, ok := e.Src["file"].(string); ok && f != "" {
		r["at"] = fmt.Sprintf("%s:%v", f, e.Src["line"])
	}
	if ts, err := time.Parse(time.RFC3339Nano, e.TS); err == nil && !d.started.IsZero() {
		r["offset_ms"] = float64(ts.Sub(d.started).Microseconds()) / 1000
	}
	return r
}

func (d *reqDetail) rows(evs []reqEvent) []any {
	out := make([]any, len(evs))
	for i, e := range evs {
		out[i] = d.row(i+1, e)
	}
	return out
}

func fetchRequestDetail(rid string) (*reqDetail, map[string]any) {
	if rid == "" {
		return nil, toolErr("rid is required; list requests to find one")
	}
	body, status, err := uiGET("/api/requests/" + url.PathEscape(rid))
	if err != nil {
		return nil, toolErr("lerd-ui not reachable: " + err.Error())
	}
	if status == http.StatusNotFound {
		return nil, toolErr(fmt.Sprintf("no request %q in the buffer; list requests to see what is there", rid))
	}
	if status != http.StatusOK {
		return nil, toolErr(fmt.Sprintf("lerd-ui returned %d: %s", status, body))
	}
	d := &reqDetail{}
	if err := json.Unmarshal(body, d); err != nil {
		return nil, toolErr("decoding request: " + err.Error())
	}
	_ = json.Unmarshal(body, &d.Head)
	for _, k := range []string{"events", "queries", "traces"} {
		delete(d.Head, k)
	}
	if s, ok := d.Head["started"].(string); ok {
		d.started, _ = time.Parse(time.RFC3339Nano, s)
	}
	return d, nil
}

// reqTab is one tab of the request view: its id and title as the Debug window
// shows them, and the rows behind it.
type reqTab struct {
	id, title string
	rows      func(*reqDetail) []any
	// extra is what a tab shows above its rows, such as the query findings.
	extra func(*reqDetail) map[string]any
}

func eventsTab(id, title string, kinds ...string) reqTab {
	return reqTab{id: id, title: title, rows: func(d *reqDetail) []any {
		var evs []reqEvent
		for _, k := range kinds {
			evs = append(evs, d.ev(k)...)
		}
		return d.rows(evs)
	}}
}

var requestTabs = []reqTab{
	{id: "performance", title: "Performance", rows: func(d *reqDetail) []any {
		return d.rows(append(append([]reqEvent{}, d.ev("span")...), d.ev("timeline")...))
	}, extra: func(d *reqDetail) map[string]any { return d.timing() }},
	{id: "request", title: "Request", rows: func(d *reqDetail) []any {
		// One row per section the Request tab shows: the route and headers,
		// the query string and body, cookies, middleware, session and user.
		var out []any
		req := d.request()
		keys := make([]string, 0, len(req))
		for k := range req {
			if k != "graphql" && k != "graphql_types" && k != "trace_ref" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, map[string]any{"section": k, "value": req[k]})
		}
		for _, k := range []string{"middleware", "session", "auth"} {
			if evs := d.ev(k); len(evs) > 0 {
				out = append(out, map[string]any{"section": k, "value": evs[len(evs)-1].Data})
			}
		}
		return out
	}},
	{id: "graphql", title: "GraphQL", rows: func(d *reqDetail) []any {
		ops, _ := d.request()["graphql"].([]any)
		return ops
	}, extra: func(d *reqDetail) map[string]any {
		return map[string]any{"types": d.request()["graphql_types"]}
	}},
	eventsTab("exceptions", "Exceptions", "exception"),
	{id: "database", title: "Database", rows: func(d *reqDetail) []any { return d.rows(d.ev("query")) }, extra: func(d *reqDetail) map[string]any {
		return map[string]any{"analysis": d.Queries}
	}},
	{id: "models", title: "Models", rows: func(d *reqDetail) []any {
		total := map[string]map[string]float64{}
		for _, e := range d.ev("models") {
			models, _ := e.Data["models"].(map[string]any)
			for model, counts := range models {
				if total[model] == nil {
					total[model] = map[string]float64{}
				}
				c, _ := counts.(map[string]any)
				for action, n := range c {
					f, _ := n.(float64)
					total[model][action] += f
				}
			}
		}
		names := make([]string, 0, len(total))
		for name := range total {
			names = append(names, name)
		}
		sort.Strings(names)
		out := make([]any, len(names))
		for i, name := range names {
			out[i] = map[string]any{"model": name, "counts": total[name]}
		}
		return out
	}},
	eventsTab("views", "Views", "view"),
	{id: "components", title: "Components", rows: func(d *reqDetail) []any {
		var order []string
		by := map[string][]any{}
		for i, e := range d.ev("component") {
			name, _ := e.Data["name"].(string)
			if _, ok := by[name]; !ok {
				order = append(order, name)
			}
			by[name] = append(by[name], d.row(i+1, e))
		}
		out := make([]any, len(order))
		for i, name := range order {
			out[i] = map[string]any{"component": name, "phases": by[name]}
		}
		return out
	}},
	eventsTab("cache", "Cache", "cache"),
	eventsTab("redis", "Redis", "redis"),
	eventsTab("filesystem", "Filesystem", "filesystem"),
	eventsTab("events", "Events", "event"),
	eventsTab("log", "Log", "log"),
	eventsTab("dumps", "Dumps", "dump"),
	eventsTab("mail", "Mail & messages", "mail", "message"),
	eventsTab("http", "HTTP", "http"),
	eventsTab("jobs", "Jobs", "job"),
	{id: "browser", title: "Browser", rows: func(d *reqDetail) []any {
		// The same events the Debug window lists: a call that reached PHP is a
		// sent request and the load timing is on the timeline.
		var evs []reqEvent
		for _, e := range d.ev("browser") {
			t, _ := e.Data["type"].(string)
			if t == "request" || t == "timing" || (t == "network" && e.Data["rid"] != nil) {
				continue
			}
			evs = append(evs, e)
		}
		return d.rows(evs)
	}},
	{id: "sent", title: "Child requests", rows: func(d *reqDetail) []any {
		kids, _ := d.Head["children"].([]any)
		return kids
	}},
}

// customTabs are the tabs the app built through lerd/debug, one per id, each
// with its blocks in the order they were added.
func (d *reqDetail) customTabs() []reqTab {
	evs := append([]reqEvent{}, d.ev("tab")...)
	sort.SliceStable(evs, func(i, j int) bool {
		a, _ := evs[i].Data["seq"].(float64)
		b, _ := evs[j].Data["seq"].(float64)
		return a < b
	})
	var order []string
	titles := map[string]string{}
	blocks := map[string][]any{}
	for _, e := range evs {
		id := fmt.Sprint(e.Data["id"])
		if _, ok := titles[id]; !ok {
			order = append(order, id)
			titles[id] = fmt.Sprint(e.Data["title"])
		}
		blocks[id] = append(blocks[id], e.Data["block"])
	}
	out := make([]reqTab, len(order))
	for i, id := range order {
		b := blocks[id]
		out[i] = reqTab{id: "custom:" + id, title: titles[id], rows: func(*reqDetail) []any { return b }}
	}
	return out
}

func (d *reqDetail) tabs() []reqTab {
	return append(append([]reqTab{}, requestTabs...), d.customTabs()...)
}

// timing is the Performance tab's headline: where the time went and the memory.
func (d *reqDetail) timing() map[string]any {
	out := map[string]any{}
	for _, k := range []string{"time_ms", "nginx_ms", "queue_ms"} {
		if v, ok := d.Head[k]; ok {
			out[k] = v
		}
	}
	req := d.request()
	if v, ok := req["memory_peak"]; ok {
		out["memory_peak"] = v
	}
	if v, ok := d.Queries["total_time_ms"]; ok {
		out["db_ms"] = v
	}
	for _, e := range d.ev("browser") {
		if e.Data["type"] == "timing" {
			out["page"] = e.Data["timing"]
		}
	}
	return out
}

func execRequestTool(args map[string]any) (any, *rpcError) {
	action := strArg(args, "action")
	if action == "list" {
		return execRequestList(args)
	}
	d, errRes := fetchRequestDetail(strArg(args, "rid"))
	if errRes != nil {
		return errRes, nil
	}
	switch action {
	case "tabs":
		var list []map[string]any
		for _, t := range d.tabs() {
			n := len(t.rows(d))
			if n == 0 && t.id != "performance" {
				continue
			}
			list = append(list, map[string]any{"id": t.id, "title": t.title, "count": n})
		}
		return toolJSON(map[string]any{"rid": d.Head["rid"], "tabs": list}), nil
	case "summary":
		return toolJSON(d.summary()), nil
	case "tab":
		id := strArg(args, "tab")
		for _, t := range d.tabs() {
			if t.id != id {
				continue
			}
			rows := t.rows(d)
			offset, limit := intArg(args, "offset", 0), intArg(args, "limit", 50)
			out := map[string]any{"tab": id, "total": len(rows), "offset": offset}
			if offset < 0 {
				offset = 0
			}
			if offset > len(rows) {
				offset = len(rows)
			}
			end := offset + limit
			if limit <= 0 || end > len(rows) {
				end = len(rows)
			}
			out["rows"] = rows[offset:end]
			if t.extra != nil && offset == 0 {
				for k, v := range t.extra(d) {
					out[k] = v
				}
			}
			return toolJSON(out), nil
		}
		ids := make([]string, 0)
		for _, t := range d.tabs() {
			ids = append(ids, t.id)
		}
		return toolErr(fmt.Sprintf("unknown tab %q (want one of %s)", id, strings.Join(ids, ", "))), nil
	case "trace":
		ref := intArg(args, "ref", -1)
		if ref < 0 || ref >= len(d.Traces) {
			return toolErr(fmt.Sprintf("no trace %d in request %v; it has %d", ref, d.Head["rid"], len(d.Traces))), nil
		}
		return toolOK(fmt.Sprintf(`{"ref":%d,"trace":%s}`, ref, d.Traces[ref])), nil
	}
	return toolErr(fmt.Sprintf("unknown action %q (want list, tabs, summary, tab or trace)", action)), nil
}

// summary is what the request view's header, the debug bar and the findings
// say about a request, small enough to read whole however much it did.
func (d *reqDetail) summary() map[string]any {
	out := map[string]any{}
	for _, k := range []string{"rid", "type", "site", "method", "uri", "route", "status", "started", "operation", "operations", "problems", "parent", "worker", "command", "job", "job_status"} {
		if v, ok := d.Head[k]; ok {
			out[k] = v
		}
	}
	out["timing"] = d.timing()
	var phases []any
	for _, e := range d.ev("span") {
		if e.Data["label"] == "View" {
			continue
		}
		phases = append(phases, map[string]any{"label": e.Data["label"], "name": e.Data["name"], "time_ms": e.Data["time_ms"], "status": e.Data["status"]})
	}
	out["phases"] = phases
	if evs := d.ev("auth"); len(evs) > 0 {
		out["user"] = evs[0].Data
	}
	var tabs []map[string]any
	for _, t := range d.tabs() {
		if n := len(t.rows(d)); n > 0 {
			tabs = append(tabs, map[string]any{"id": t.id, "count": n})
		}
	}
	out["tabs"] = tabs
	if d.Queries != nil {
		out["database"] = map[string]any{"count": d.Queries["query_count"], "time_ms": d.Queries["total_time_ms"], "n_plus_one": withoutIDs(d.Queries["n_plus_one"]), "slow": d.Queries["slow"]}
	}
	var exceptions []any
	for _, e := range d.ev("exception") {
		exceptions = append(exceptions, map[string]any{"type": e.Data["type"], "message": e.Data["message"], "at": d.row(0, e)["at"]})
	}
	out["exceptions"] = exceptions
	var ops []any
	gql, _ := d.request()["graphql"].([]any)
	for _, op := range gql {
		o, _ := op.(map[string]any)
		var fields []any
		f, _ := o["fields"].([]any)
		for _, x := range f {
			if fm, ok := x.(map[string]any); ok {
				fields = append(fields, map[string]any{"name": fm["name"], "type": fm["type"]})
			}
		}
		ops = append(ops, map[string]any{"type": o["type"], "name": o["name"], "fields": fields, "errors": o["errors"]})
	}
	out["graphql"] = ops
	out["sent"] = d.Head["children"]
	return out
}

// withoutIDs drops the query ids from N+1 findings, which only the tab's rows need.
func withoutIDs(v any) any {
	list, _ := v.([]any)
	out := make([]any, len(list))
	for i, f := range list {
		m, _ := f.(map[string]any)
		c := map[string]any{}
		for k, x := range m {
			if k != "ids" {
				c[k] = x
			}
		}
		out[i] = c
	}
	return out
}
