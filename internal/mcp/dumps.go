package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumpsops"
)

// execDumpsRecent calls lerd-ui's /api/dumps endpoint over the local transport
// (unix socket on Linux, TCP loopback on macOS) and returns the JSON response
// verbatim. We don't reach into the
// in-process ring directly because the MCP server may run in a different
// process from lerd-ui (e.g. an editor-launched MCP subprocess).
func execDumpsRecent(args map[string]any) (any, *rpcError) {
	q := []string{}
	if s := strArg(args, "site"); s != "" {
		q = append(q, "site="+s)
	}
	if b := strArg(args, "branch"); b != "" {
		q = append(q, "branch="+b)
	}
	if c := strArg(args, "ctx"); c != "" {
		if c != "fpm" && c != "cli" {
			return toolErr(`ctx must be "fpm" or "cli"`), nil
		}
		q = append(q, "ctx="+c)
	}
	if k := strArg(args, "kind"); k != "" {
		q = append(q, "kind="+k)
	}
	if s := strArg(args, "since"); s != "" {
		q = append(q, "since="+s)
	}
	if limit, ok := args["limit"]; ok {
		q = append(q, fmt.Sprintf("limit=%v", limit))
	}
	path := "/api/dumps"
	if len(q) > 0 {
		path += "?" + strings.Join(q, "&")
	}
	body, status, err := uiGET(path)
	if err != nil {
		return toolErr("lerd-ui not reachable: " + err.Error()), nil
	}
	if status != http.StatusOK {
		return toolErr(fmt.Sprintf("lerd-ui returned %d: %s", status, body)), nil
	}
	return toolOK(string(body)), nil
}

// execAnalyzeQueries calls lerd-ui's /api/queries/analyze endpoint, returning
// the N+1 / slow-query report verbatim. Lives next to dumps_recent because it
// reads the same captured-query ring; the analysis itself is server-side so the
// fingerprinting matches the dashboard and the N+1 notifications.
func execAnalyzeQueries(args map[string]any) (any, *rpcError) {
	params := [][2]string{{"site", strArg(args, "site")}}
	if v, ok := args["min_repeat"]; ok {
		params = append(params, [2]string{"min_repeat", fmt.Sprintf("%v", v)})
	}
	if v, ok := args["slow_ms"]; ok {
		params = append(params, [2]string{"slow_ms", fmt.Sprintf("%v", v)})
	}
	path := queryPath("/api/queries/analyze", params)
	body, status, err := uiGET(path)
	if err != nil {
		return toolErr("lerd-ui not reachable: " + err.Error()), nil
	}
	if status != http.StatusOK {
		return toolErr(fmt.Sprintf("lerd-ui returned %d: %s", status, body)), nil
	}
	return toolOK(string(body)), nil
}

// execRouteTiming calls lerd-ui's /api/queries/route-timing endpoint, returning
// the per-site request-timing snapshot (median response time and the routes
// whose p95 runs well above it) verbatim. This is the timing table the doctor's
// slow_routes finding only summarizes in prose.
func execRouteTiming(args map[string]any) (any, *rpcError) {
	path := queryPath("/api/queries/route-timing", [][2]string{
		{"site", strArg(args, "site")},
		{"branch", strArg(args, "branch")},
	})
	body, status, err := uiGET(path)
	if err != nil {
		return toolErr("lerd-ui not reachable: " + err.Error()), nil
	}
	if status != http.StatusOK {
		return toolErr(fmt.Sprintf("lerd-ui returned %d: %s", status, body)), nil
	}
	return toolOK(string(body)), nil
}

// execOptimizeRoute calls lerd-ui's /api/queries/optimize endpoint, returning the
// joined view: each slow route alongside the N+1 and slow-query findings captured
// against it, so an agent gets the symptom and its cause in one call rather than
// pivoting between route_timing and analyze_queries by hand.
func execOptimizeRoute(args map[string]any) (any, *rpcError) {
	params := [][2]string{
		{"site", strArg(args, "site")},
		{"branch", strArg(args, "branch")},
	}
	if v, ok := args["min_repeat"]; ok {
		params = append(params, [2]string{"min_repeat", fmt.Sprintf("%v", v)})
	}
	if v, ok := args["slow_ms"]; ok {
		params = append(params, [2]string{"slow_ms", fmt.Sprintf("%v", v)})
	}
	path := queryPath("/api/queries/optimize", params)
	body, status, err := uiGET(path)
	if err != nil {
		return toolErr("lerd-ui not reachable: " + err.Error()), nil
	}
	if status != http.StatusOK {
		return toolErr(fmt.Sprintf("lerd-ui returned %d: %s", status, body)), nil
	}
	return toolOK(string(body)), nil
}

func execDumpsStatus(_ map[string]any) (any, *rpcError) {
	body, status, err := uiGET("/api/dumps/status")
	if err != nil {
		// MCP shouldn't fail loudly when lerd-ui is down — return a sensible
		// JSON snapshot derived from config alone.
		cfg, cerr := config.LoadGlobal()
		if cerr != nil {
			return toolErr("lerd-ui not reachable: " + err.Error()), nil
		}
		snap := map[string]any{
			"enabled":   cfg.IsDumpsEnabled(),
			"listening": false,
			"reason":    err.Error(),
		}
		b, _ := json.Marshal(snap)
		return toolOK(string(b)), nil
	}
	if status != http.StatusOK {
		return toolErr(fmt.Sprintf("lerd-ui returned %d: %s", status, body)), nil
	}
	return toolOK(string(body)), nil
}

func execDumpsClear(_ map[string]any) (any, *rpcError) {
	_, status, err := uiPOST("/api/dumps/clear", nil)
	if err != nil {
		return toolErr("lerd-ui not reachable: " + err.Error()), nil
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return toolErr(fmt.Sprintf("lerd-ui returned %d", status)), nil
	}
	return toolOK(`{"ok":true}`), nil
}

func execDumpsToggle(args map[string]any) (any, *rpcError) {
	enableRaw, ok := args["enable"]
	if !ok {
		return toolErr(`"enable" is required (true or false)`), nil
	}
	enable, ok := enableRaw.(bool)
	if !ok {
		return toolErr(`"enable" must be a boolean`), nil
	}
	res, err := dumpsops.Apply(enable)
	if err != nil {
		return toolErr("toggle failed: " + err.Error()), nil
	}
	b, _ := json.Marshal(res)
	return toolOK(string(b)), nil
}

// queryPath joins base with the non-empty params as an escaped, key-sorted query
// string. Escaping matters because a value like a git branch name can carry &,
// =, # or +, which raw concatenation would splice into the query.
func queryPath(base string, params [][2]string) string {
	q := url.Values{}
	for _, p := range params {
		if p[1] != "" {
			q.Set(p[0], p[1])
		}
	}
	if enc := q.Encode(); enc != "" {
		return base + "?" + enc
	}
	return base
}

// uiGET / uiPOST: tiny HTTP helpers over the OS-appropriate lerd-ui transport
// (unix socket on Linux, TCP loopback on macOS). Local to mcp so callers don't
// have to import a heavier client. uiRoundTrip is swappable so tests can assert
// the path/body an exec builds without a live lerd-ui daemon.
var uiRoundTrip = uiDo

func uiGET(path string) ([]byte, int, error) {
	req, _ := http.NewRequest("GET", "http://lerd"+path, nil)
	return uiRoundTrip(req)
}

func uiPOST(path string, body []byte) ([]byte, int, error) {
	req, _ := http.NewRequest("POST", "http://lerd"+path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return uiRoundTrip(req)
}

// uiClientDial reports the transport used to reach the lerd-ui daemon: the unix
// socket on Linux, the TCP loopback on macOS where the socket isn't created. A
// var so tests can point it at a fake listener regardless of the per-OS default.
var uiClientDial = func() (network, addr string) {
	return config.UIClientNetwork(), config.UIClientAddr()
}

func uiDo(req *http.Request) ([]byte, int, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				network, addr := uiClientDial()
				return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, addr)
			},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}
