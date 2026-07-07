package mcp

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
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

// TestUiDo_DialsConfiguredTransport confirms the dump round-trip honors the
// OS-appropriate transport from config (TCP loopback on macOS, where the lerd-ui
// unix socket is never created) rather than a hardcoded unix socket.
func TestUiDo_DialsConfiguredTransport(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var gotPath atomic.Pointer[string]
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		gotPath.Store(&p)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close(); _ = ln.Close() })

	prev := uiClientDial
	uiClientDial = func() (string, string) { return "tcp", ln.Addr().String() }
	t.Cleanup(func() { uiClientDial = prev })

	body, status, err := uiGET("/api/dumps")
	if err != nil {
		t.Fatalf("uiGET over tcp: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
	if got := gotPath.Load(); got == nil || *got != "/api/dumps" {
		t.Fatalf("server did not receive request over tcp transport")
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

// A branch name with query-significant characters must be escaped, not spliced
// raw into the query string where & or = would corrupt the request.
func TestExecRouteTiming_EscapesBranch(t *testing.T) {
	path := stubRoundTrip(t, `{}`)
	if _, rpcErr := execRouteTiming(map[string]any{"site": "acme", "branch": "feat/a&b=c"}); rpcErr != nil {
		t.Fatalf("rpcErr: %v", rpcErr)
	}
	if strings.Contains(*path, "a&b=c") {
		t.Errorf("branch was not escaped: %q", *path)
	}
	if !strings.Contains(*path, "site=acme") {
		t.Errorf("site param lost: %q", *path)
	}
	// The escaped branch must round-trip back to the original value.
	u, err := url.Parse("http://x" + *path)
	if err != nil {
		t.Fatalf("path not parseable: %v", err)
	}
	if got := u.Query().Get("branch"); got != "feat/a&b=c" {
		t.Errorf("branch decoded to %q, want %q", got, "feat/a&b=c")
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
