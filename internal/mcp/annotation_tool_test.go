package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAnnotationTool_ListsGetsAndResolves(t *testing.T) {
	var posted string
	orig := uiRoundTrip
	uiRoundTrip = func(req *http.Request) ([]byte, int, error) {
		switch {
		case req.Method == "GET" && req.URL.Path == "/api/annotations":
			if req.URL.Query().Get("site") != "shop" {
				t.Errorf("site %q", req.URL.Query().Get("site"))
			}
			return []byte(`[{"id":"a1","site":"shop","comment":"typo","selector":"h1","status":"open","rid":"r1"}]`), 200, nil
		case req.Method == "GET" && req.URL.Path == "/api/annotations/shop/a1":
			return []byte(`{"id":"a1","site":"shop","comment":"typo","selector":"h1","status":"open","rid":"r1"}`), 200, nil
		case req.Method == "POST" && req.URL.Path == "/api/annotations/shop/a1":
			b, _ := io.ReadAll(req.Body)
			posted = string(b)
			return []byte(`{"id":"a1","status":"resolved"}`), 200, nil
		}
		return []byte("nope"), 404, nil
	}
	t.Cleanup(func() { uiRoundTrip = orig })

	res, _ := execAnnotationTool(map[string]any{"action": "list", "site": "shop"})
	if b, _ := json.Marshal(res); !strings.Contains(string(b), `typo`) || !strings.Contains(string(b), `r1`) {
		t.Errorf("list %s", b)
	}
	res, _ = execAnnotationTool(map[string]any{"action": "get", "site": "shop", "id": "a1"})
	if b, _ := json.Marshal(res); !strings.Contains(string(b), `request`) {
		t.Errorf("get should point at the request tool for the page view: %s", b)
	}
	if _, rpcErr := execAnnotationTool(map[string]any{"action": "resolve", "site": "shop", "id": "a1", "resolution": "Fixed"}); rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if !strings.Contains(posted, `"status":"resolved"`) || !strings.Contains(posted, `"resolution":"Fixed"`) {
		t.Errorf("posted %s", posted)
	}
	if res, _ := execAnnotationTool(map[string]any{"action": "resolve", "site": "shop"}); !strings.Contains(func() string { b, _ := json.Marshal(res); return string(b) }(), "isError") {
		t.Error("resolve without id accepted")
	}
}
