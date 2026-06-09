package mcp

import (
	"net/http"
	"strings"
	"testing"
)

var dumpActionNames = []string{"dumps_recent", "analyze_queries", "dumps_status", "dumps_clear", "dumps_toggle"}

// TestDumpActions_RoutableUnderDiag confirms the dump/query actions are reachable
// as actions on the consolidated diag tool.
func TestDumpActions_RoutableUnderDiag(t *testing.T) {
	diag := groupDispatch["diag"]
	for _, want := range dumpActionNames {
		if _, ok := diag[want]; !ok {
			t.Errorf("diag tool missing action %q", want)
		}
	}
}

// stubRoundTrip swaps the MCP HTTP round-trip for one that records the request
// path and returns a canned body, so an exec's URL building can be asserted
// without a live lerd-ui socket.
func stubRoundTrip(t *testing.T, body string) *string {
	t.Helper()
	prev := uiRoundTrip
	var gotPath string
	uiRoundTrip = func(req *http.Request) ([]byte, int, error) {
		gotPath = req.URL.RequestURI()
		return []byte(body), http.StatusOK, nil
	}
	t.Cleanup(func() { uiRoundTrip = prev })
	return &gotPath
}

func TestExecAnalyzeQueries_BuildsQueryAndPassesBody(t *testing.T) {
	path := stubRoundTrip(t, `{"summary":{"n_plus_one_findings":2}}`)
	got, rpcErr := execAnalyzeQueries(map[string]any{"site": "acme", "min_repeat": 5, "slow_ms": 50})
	if rpcErr != nil {
		t.Fatalf("rpcErr: %v", rpcErr)
	}
	if !strings.HasPrefix(*path, "/api/queries/analyze?") {
		t.Errorf("path = %q, want /api/queries/analyze?…", *path)
	}
	for _, frag := range []string{"site=acme", "min_repeat=5", "slow_ms=50"} {
		if !strings.Contains(*path, frag) {
			t.Errorf("path %q missing %q", *path, frag)
		}
	}
	if !strings.Contains(toolText(got), "n_plus_one_findings") {
		t.Errorf("body not passed through: %q", toolText(got))
	}
}

func TestExecDumpsRecent_KindPassedThrough(t *testing.T) {
	path := stubRoundTrip(t, `[]`)
	if _, rpcErr := execDumpsRecent(map[string]any{"kind": "query", "site": "acme"}); rpcErr != nil {
		t.Fatalf("rpcErr: %v", rpcErr)
	}
	if !strings.Contains(*path, "kind=query") || !strings.Contains(*path, "site=acme") {
		t.Errorf("path %q missing kind/site", *path)
	}
}

func TestExecDumpsRecent_BranchPassedThrough(t *testing.T) {
	path := stubRoundTrip(t, `[]`)
	if _, rpcErr := execDumpsRecent(map[string]any{"site": "acme", "branch": "feature-x"}); rpcErr != nil {
		t.Fatalf("rpcErr: %v", rpcErr)
	}
	if !strings.Contains(*path, "branch=feature-x") {
		t.Errorf("path %q missing branch filter", *path)
	}
}

func TestDumpsToggle_RequiresEnable(t *testing.T) {
	got, rpcErr := execDumpsToggle(map[string]any{})
	if rpcErr != nil {
		t.Fatalf("unexpected rpcErr: %v", rpcErr)
	}
	body := toolText(got)
	if !strings.Contains(body, "required") {
		t.Errorf("expected error about required enable, got %q", body)
	}
}

func TestDumpsToggle_RejectsWrongType(t *testing.T) {
	got, _ := execDumpsToggle(map[string]any{"enable": "yes"})
	body := toolText(got)
	if !strings.Contains(body, "boolean") {
		t.Errorf("expected type error, got %q", body)
	}
}

func TestDumpsRecent_RejectsBadCtx(t *testing.T) {
	got, _ := execDumpsRecent(map[string]any{"ctx": "queue"})
	body := toolText(got)
	if !strings.Contains(body, `"fpm"`) {
		t.Errorf("expected ctx validation message, got %q", body)
	}
}

// toolText extracts the text payload from a tool response without enforcing
// schema (handles both OK and error shapes).
func toolText(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	c, ok := m["content"].([]map[string]any)
	if !ok {
		return ""
	}
	if len(c) == 0 {
		return ""
	}
	t, _ := c[0]["text"].(string)
	return t
}
