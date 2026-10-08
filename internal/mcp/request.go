package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func requestTool() mcpTool {
	return mcpTool{
		Name:        "request",
		Description: "One request's Debug lenses, by rid. list: recent requests; lenses: counts, PHP cost, issues (N+1, errors); lens: one lens, paged.",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"action": {Type: "string", Enum: []string{"list", "lenses", "lens"}},
				"rid":    {Type: "string"},
				"lens":   {Type: "string", Enum: requestLensNames()},
				"site":   {Type: "string"},
				"branch": {Type: "string"},
				"offset": {Type: "integer"},
				"limit":  {Type: "integer"},
			},
			Required: []string{"action"},
		},
	}
}

// requestLenses maps the Debug window's lens names to the event kind each
// shows, so an assistant reads a request the way the dashboard does.
var requestLenses = []struct{ lens, kind string }{
	{"exceptions", "exception"}, {"logs", "log"}, {"browser", "browser"}, {"dumps", "dump"},
	{"queries", "query"}, {"views", "view"}, {"cache", "cache"}, {"http", "http"},
	{"jobs", "job"}, {"messages", "message"}, {"events", "event"}, {"mail", "mail"},
}

func requestLensNames() []string {
	out := make([]string, len(requestLenses))
	for i, l := range requestLenses {
		out[i] = l.lens
	}
	return out
}

func lensKind(lens string) string {
	for _, l := range requestLenses {
		if l.lens == lens {
			return l.kind
		}
	}
	return ""
}

// reqEvent is a captured event as /api/dumps serves it, read loosely.
type reqEvent struct {
	TS    string         `json:"ts"`
	Kind  string         `json:"kind"`
	Ctx   map[string]any `json:"ctx"`
	Src   map[string]any `json:"src"`
	Label string         `json:"label"`
	Text  string         `json:"text"`
	Data  map[string]any `json:"data"`
}

func execRequestTool(args map[string]any) (any, *rpcError) {
	switch strArg(args, "action") {
	case "list":
		return execRequestList(args)
	case "lenses":
		evs, errOut := requestEvents(strArg(args, "rid"), "")
		if errOut != nil {
			return errOut, nil
		}
		issues, errOut := requestQueryIssues(strArg(args, "rid"))
		if errOut != nil {
			return errOut, nil
		}
		out := requestLensCounts(evs)
		out["issues"] = append(issues, requestLensIssues(evs)...)
		return jsonOK(out), nil
	case "lens":
		kind := lensKind(strArg(args, "lens"))
		if kind == "" {
			return toolErr("lens is required; lenses lists what the request has"), nil
		}
		// Every lens reads offsets from the request's first event, so the whole
		// request is read and the lens taken from it.
		evs, errOut := requestEvents(strArg(args, "rid"), "")
		if errOut != nil {
			return errOut, nil
		}
		return jsonOK(requestLensPage(evs, kind, intArg(args, "offset", 0), intArg(args, "limit", 50))), nil
	}
	return toolErr("unknown action for tool \"request\""), nil
}

// execRequestList lists a site's recent requests from the timing view, each
// with the id its captured events are grouped under.
func execRequestList(args map[string]any) (any, *rpcError) {
	site, errOut := resolveSiteForWorktree(args)
	if errOut != nil {
		return errOut, nil
	}
	path := queryPath("/api/sites/"+url.PathEscape(site.PrimaryDomain())+"/analytics", [][2]string{{"range", "1h"}, {"branch", branchFromArgs(args, site)}})
	body, status, err := uiGET(path)
	if err != nil {
		return toolErr("lerd-ui not reachable: " + err.Error()), nil
	}
	if status != http.StatusOK {
		return toolErr(fmt.Sprintf("lerd-ui returned %d: %s", status, body)), nil
	}
	var a struct {
		Recent []map[string]any `json:"recent"`
	}
	_ = json.Unmarshal(body, &a)
	a.Recent = recentInLastHour(a.Recent, time.Now(), max(intArg(args, "limit", 20), 1))
	out := map[string]any{"site": site.Name, "requests": a.Recent}
	if len(a.Recent) == 0 {
		out["hint"] = "No requests in the last hour. Load a page on the site and list again."
	} else if _, ok := a.Recent[0]["rid"]; !ok {
		out["hint"] = "Requests without a rid ran while debug capture was off; turn it on with diag dumps_toggle."
	}
	return jsonOK(out), nil
}

// recentInLastHour keeps the newest recent rows from the last hour, at most
// limit of them; the analytics range bounds its figures, not this list.
func recentInLastHour(rows []map[string]any, now time.Time, limit int) []map[string]any {
	since := float64(now.Add(-time.Hour).UnixMilli())
	out := make([]map[string]any, 0, min(len(rows), limit))
	for _, r := range rows {
		if at, _ := r["at_millis"].(float64); at < since || len(out) == limit {
			break
		}
		out = append(out, r)
	}
	return out
}

func requestEvents(rid, kind string) ([]reqEvent, map[string]any) {
	if rid == "" {
		return nil, toolErr("rid is required; list requests to find one")
	}
	body, status, err := uiGET(queryPath("/api/dumps", [][2]string{{"rid", rid}, {"kind", kind}}))
	if err != nil {
		return nil, toolErr("lerd-ui not reachable: " + err.Error())
	}
	if status != http.StatusOK {
		return nil, toolErr(fmt.Sprintf("lerd-ui returned %d: %s", status, body))
	}
	var evs []reqEvent
	if err := json.Unmarshal(body, &evs); err != nil {
		return nil, toolErr("decoding events: " + err.Error())
	}
	return evs, nil
}

// requestLensCounts is what the request's lens bar shows: who served it and
// how many events each lens holds. A page view is the page itself, not an event.
func requestLensCounts(evs []reqEvent) map[string]any {
	counts := map[string]int{}
	out := map[string]any{"lenses": counts}
	for _, e := range evs {
		if out["site"] == nil && e.Kind != "browser" {
			out["site"], out["request"] = e.Ctx["site"], e.Ctx["request"]
		}
		if e.Kind == "browser" && e.Data["type"] == "navigation" {
			continue
		}
		if e.Kind == "request" {
			delete(e.Data, "trace")
			out["php"] = e.Data
			continue
		}
		for _, l := range requestLenses {
			if l.kind == e.Kind {
				counts[l.lens]++
			}
		}
	}
	if len(evs) == 0 {
		out["hint"] = "Nothing captured for this id; it may have left the buffer."
	}
	return out
}

// requestQueryIssues reads the request's N+1 and slow queries from the same
// analyzer diag analyze_queries runs over all traffic, so the two never disagree.
func requestQueryIssues(rid string) ([]map[string]any, map[string]any) {
	body, status, err := uiGET(queryPath("/api/queries/analyze", [][2]string{{"rid", rid}}))
	if err != nil {
		return nil, toolErr("lerd-ui not reachable: " + err.Error())
	}
	if status != http.StatusOK {
		return nil, toolErr(fmt.Sprintf("lerd-ui returned %d: %s", status, body))
	}
	type caller struct {
		File string `json:"file"`
		Line int    `json:"line"`
	}
	var a struct {
		Requests []struct {
			NPlusOne []struct {
				Count     int     `json:"count"`
				TotalMS   float64 `json:"total_time_ms"`
				SampleSQL string  `json:"sample_sql"`
				Caller    caller  `json:"caller"`
			} `json:"n_plus_one"`
			Slow []struct {
				SQL    string  `json:"sql"`
				TimeMS float64 `json:"time_ms"`
				Caller caller  `json:"caller"`
			} `json:"slow"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(body, &a); err != nil {
		return nil, toolErr("decoding query analysis: " + err.Error())
	}
	at := func(c caller) string { return fmt.Sprintf("%s:%d", c.File, c.Line) }
	issues := []map[string]any{}
	for _, r := range a.Requests {
		for _, n := range r.NPlusOne {
			issues = append(issues, map[string]any{"lens": "queries", "issue": "n_plus_one", "count": n.Count, "total_ms": n.TotalMS, "sql": n.SampleSQL, "at": at(n.Caller)})
		}
		for _, q := range r.Slow {
			issues = append(issues, map[string]any{"lens": "queries", "issue": "slow", "time_ms": q.TimeMS, "sql": q.SQL, "at": at(q.Caller)})
		}
	}
	return issues, nil
}

// lensIssue names what is wrong with one event, using the dashboard's rose and
// amber tones as the bar: anything it paints neutral is not an issue.
func lensIssue(e reqEvent) string {
	str := func(k string) string { v, _ := e.Data[k].(string); return v }
	switch e.Kind {
	case "exception":
		return "error"
	case "log":
		switch str("level") {
		case "emergency", "alert", "critical", "error":
			return "error"
		case "warning", "notice":
			return "warning"
		}
	case "browser":
		t := str("type")
		if t == "error" || t == "rejection" || (t == "console" && str("level") == "error") {
			return "error"
		}
		if t == "console" && str("level") == "warn" {
			return "warning"
		}
		// A fetch that got no response is stored without a status at all.
		if code, _ := e.Data["status"].(float64); t == "network" && (code == 0 || code >= 400) {
			return "failed"
		}
	case "http":
		if code, _ := e.Data["status"].(float64); e.Data["failed"] == true || code >= 400 {
			return "failed"
		}
	case "job":
		if str("status") == "failed" {
			return "failed"
		}
	}
	return ""
}

// requestLensIssues folds every flagged event into one row per lens and issue,
// counted, with the first occurrence to start reading from.
func requestLensIssues(evs []reqEvent) []map[string]any {
	var out []map[string]any
	seen := map[string]map[string]any{}
	for _, e := range evs {
		issue := lensIssue(e)
		if issue == "" {
			continue
		}
		key := e.Kind + "/" + issue
		if row := seen[key]; row != nil {
			row["count"] = row["count"].(int) + 1
			continue
		}
		row := map[string]any{"lens": lensOfKind(e.Kind), "issue": issue, "count": 1}
		if first := issueText(e); first != "" {
			row["first"] = first
		}
		if f, ok := e.Src["file"].(string); ok && f != "" {
			row["at"] = fmt.Sprintf("%s:%v", f, e.Src["line"])
		}
		seen[key] = row
		out = append(out, row)
	}
	return out
}

// issueText is the line the dashboard's lens row shows for the event.
func issueText(e reqEvent) string {
	str := func(k string) string { v, _ := e.Data[k].(string); return v }
	switch {
	case e.Kind == "job":
		return str("class")
	case e.Kind == "http":
		return withStatus(str("method")+" "+str("url"), e.Data["status"])
	case e.Kind == "browser" && str("type") == "network":
		return withStatus(str("method")+" "+str("request"), e.Data["status"])
	case e.Kind == "exception" && str("type") != "" && str("type") != "message":
		return str("type") + " " + str("message")
	}
	return str("message")
}

func withStatus(call string, status any) string {
	if status == nil {
		return strings.TrimSpace(call)
	}
	return strings.TrimSpace(fmt.Sprintf("%s %v", call, status))
}

func lensOfKind(kind string) string {
	for _, l := range requestLenses {
		if l.kind == kind {
			return l.lens
		}
	}
	return kind
}

// requestLensPage is one page of a lens's rows, oldest first, each placed on
// the clock of the request's first event and stripped of its stack trace.
func requestLensPage(all []reqEvent, kind string, offset, limit int) map[string]any {
	var start time.Time
	if len(all) > 0 {
		start, _ = time.Parse(time.RFC3339Nano, all[0].TS)
	}
	var evs []reqEvent
	for _, e := range all {
		if e.Kind == kind {
			evs = append(evs, e)
		}
	}
	total := len(evs)
	evs = evs[min(max(offset, 0), total):]
	if limit > 0 && len(evs) > limit {
		evs = evs[:limit]
	}
	rows := make([]map[string]any, 0, len(evs))
	for i, e := range evs {
		r := map[string]any{"n": offset + i + 1}
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
		if ts, err := time.Parse(time.RFC3339Nano, e.TS); err == nil && !start.IsZero() {
			r["offset_ms"] = float64(ts.Sub(start).Microseconds()) / 1000
		}
		rows = append(rows, r)
	}
	return map[string]any{"total": total, "rows": rows}
}

func jsonOK(v any) map[string]any {
	b, _ := json.Marshal(v)
	return toolOK(string(b))
}
