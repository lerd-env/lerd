package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const requestFixture = `{
  "rid": "r1", "type": "page", "method": "POST", "uri": "/graphql", "status": 200, "time_ms": 40, "nginx_ms": 1, "queue_ms": 2,
  "started": "2026-10-04T10:00:00.000Z", "counts": {"query": 3}, "problems": ["N+1"], "operation": "query Team",
  "children": [{"rid": "c1", "url": "/api/x", "via": "fetch", "status": 500, "duration_ms": 12}],
  "queries": {"query_count": 3, "total_time_ms": 3, "n_plus_one": [{"fingerprint": "select * from users where id = ?", "count": 3, "total_time_ms": 3, "example_sql": "select * from users where id = 1", "caller": {"file": "/app/a.php", "line": 9}, "ids": ["q0", "q1", "q2"]}]},
  "traces": [[{"file": "/app/a.php", "line": 9, "func": "load"}]],
  "events": {
    "request": [{"id": "e0", "ts": "2026-10-04T10:00:00.040Z", "kind": "request", "src": {}, "data": {"method": "POST", "memory_peak": 4194304, "graphql": [{"type": "query", "name": "Team", "query": "query Team { users { id } }", "fields": [{"name": "users", "type": "[User!]!"}]}], "graphql_types": {"User": {"kind": "object"}}}}],
    "span": [{"id": "s0", "ts": "2026-10-04T10:00:00.030Z", "kind": "span", "src": {}, "data": {"label": "Controller", "name": "GraphQLController@query", "time_ms": 20, "status": "ok"}}],
    "query": [
      {"id": "q0", "ts": "2026-10-04T10:00:00.010Z", "kind": "query", "src": {"file": "/app/a.php", "line": 9}, "data": {"sql": "select * from users where id = 1", "time_ms": 1, "trace_ref": 0}},
      {"id": "q1", "ts": "2026-10-04T10:00:00.011Z", "kind": "query", "src": {"file": "/app/a.php", "line": 9}, "data": {"sql": "select * from users where id = 2", "time_ms": 1, "trace_ref": 0}},
      {"id": "q2", "ts": "2026-10-04T10:00:00.012Z", "kind": "query", "src": {"file": "/app/a.php", "line": 9}, "data": {"sql": "select * from users where id = 3", "time_ms": 1, "trace_ref": 0}}
    ],
    "tab": [
      {"id": "t0", "ts": "2026-10-04T10:00:00.020Z", "kind": "tab", "src": {}, "data": {"id": "cart", "title": "Cart", "seq": 1, "block": {"type": "table", "rows": [["total", "19.90"]]}}},
      {"id": "t1", "ts": "2026-10-04T10:00:00.021Z", "kind": "tab", "src": {}, "data": {"id": "cart", "title": "Cart", "seq": 2, "block": {"type": "text", "text": "paid"}}}
    ]
  }
}`

func stubRequestDetail(t *testing.T) {
	t.Helper()
	orig := uiRoundTrip
	uiRoundTrip = func(req *http.Request) ([]byte, int, error) {
		if req.URL.Path == "/api/requests/r2" {
			return []byte(`{"rid": "r2", "started": "2026-10-04T10:00:00.000Z", "counts": {}, "problems": [], "events": {"request": [{"id": "e0", "ts": "2026-10-04T10:00:00.010Z", "kind": "request", "src": {}, "data": {"method": "GET"}}]}}`), http.StatusOK, nil
		}
		if req.URL.Path != "/api/requests/r1" {
			return []byte("not found"), http.StatusNotFound, nil
		}
		return []byte(requestFixture), http.StatusOK, nil
	}
	t.Cleanup(func() { uiRoundTrip = orig })
}

func callRequestTool(t *testing.T, args map[string]any) map[string]any {
	t.Helper()
	res, rpcErr := execRequestTool(args)
	if rpcErr != nil {
		t.Fatalf("rpc error: %v", rpcErr)
	}
	text := res.(map[string]any)["content"].([]map[string]any)[0]["text"].(string)
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("%v: %s", err, text)
	}
	return out
}

// The tabs a request has are the ones the Debug window shows, the app's own included.
func TestRequestTool_ListsTheTabsTheAppAddedToo(t *testing.T) {
	stubRequestDetail(t)
	out := callRequestTool(t, map[string]any{"action": "tabs", "rid": "r1"})
	raw, _ := json.Marshal(out["tabs"])
	for _, want := range []string{`"id":"performance"`, `"count":1,"id":"graphql","title":"GraphQL"`, `"count":3,"id":"database","title":"Database"`, `"id":"sent"`, `"count":2,"id":"custom:cart","title":"Cart"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %s in %s", want, raw)
		}
	}
	if !strings.Contains(string(raw), `"count":2,"id":"request","title":"Request"`) {
		t.Errorf("the request tab counts its sections: %s", raw)
	}
	if strings.Contains(string(raw), `"id":"views"`) {
		t.Errorf("an empty tab listed: %s", raw)
	}
}

// A summary carries the timing, the findings and the GraphQL operations, not every event.
func TestRequestTool_SummaryHasTheFindingsAndNoEvents(t *testing.T) {
	stubRequestDetail(t)
	out := callRequestTool(t, map[string]any{"action": "summary", "rid": "r1"})
	raw, _ := json.Marshal(out)
	for _, want := range []string{`"example_sql":"select * from users where id = 1"`, `"Team"`, `"memory_peak":4194304`, `"rid":"c1"`, `GraphQLController@query`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %s in %s", want, raw)
		}
	}
	if _, ok := out["events"]; ok {
		t.Errorf("summary carries the events")
	}
}

// A request with no GraphQL, queries or findings still summarises.
func TestRequestTool_SummarisesAPlainRequest(t *testing.T) {
	stubRequestDetail(t)
	out := callRequestTool(t, map[string]any{"action": "summary", "rid": "r2"})
	if out["rid"] != "r2" {
		t.Errorf("summary %v", out)
	}
}

// One tab's rows come a page at a time with the total, and a custom tab its blocks.
func TestRequestTool_PagesATab(t *testing.T) {
	stubRequestDetail(t)
	out := callRequestTool(t, map[string]any{"action": "tab", "rid": "r1", "tab": "database", "offset": 1, "limit": 1})
	rows := out["rows"].([]any)
	if out["total"].(float64) != 3 || len(rows) != 1 || rows[0].(map[string]any)["n"].(float64) != 2 || rows[0].(map[string]any)["sql"] != "select * from users where id = 2" {
		t.Errorf("page %v", out)
	}
	cart := callRequestTool(t, map[string]any{"action": "tab", "rid": "r1", "tab": "custom:cart"})
	if raw, _ := json.Marshal(cart); !strings.Contains(string(raw), `"text":"paid"`) {
		t.Errorf("custom tab %s", raw)
	}
}

func TestRequestTool_ReadsATraceByItsRef(t *testing.T) {
	stubRequestDetail(t)
	out := callRequestTool(t, map[string]any{"action": "trace", "rid": "r1", "ref": 0})
	if raw, _ := json.Marshal(out); !strings.Contains(string(raw), `"func":"load"`) {
		t.Errorf("trace %s", raw)
	}
}

func TestRequestTool_RefusesAnUnknownTab(t *testing.T) {
	stubRequestDetail(t)
	res, _ := execRequestTool(map[string]any{"action": "tab", "rid": "r1", "tab": "nope"})
	if b, _ := json.Marshal(res); !strings.Contains(string(b), `nope`) || !strings.Contains(string(b), `isError`) {
		t.Errorf("unknown tab not refused: %s", b)
	}
}
