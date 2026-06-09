package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumpsops"
)

// execDumpsRecent calls lerd-ui's /api/dumps endpoint over the local Unix
// socket and returns the JSON response verbatim. We don't reach into the
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
	q := []string{}
	if s := strArg(args, "site"); s != "" {
		q = append(q, "site="+s)
	}
	if v, ok := args["min_repeat"]; ok {
		q = append(q, fmt.Sprintf("min_repeat=%v", v))
	}
	if v, ok := args["slow_ms"]; ok {
		q = append(q, fmt.Sprintf("slow_ms=%v", v))
	}
	path := "/api/queries/analyze"
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

// uiGET / uiPOST: tiny HTTP-over-Unix-socket helpers. Local to mcp so callers
// don't have to import a heavier client. uiRoundTrip is swappable so tests can
// assert the path/body an exec builds without a live lerd-ui socket.
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

func uiDo(req *http.Request) ([]byte, int, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: 2 * time.Second}
				return d.DialContext(ctx, "unix", config.UISocketPath())
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
