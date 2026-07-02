package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// callToolRaw invokes handleToolCall the way an MCP client would: a tool name
// plus a JSON arguments object.
func callToolRaw(t *testing.T, tool string, args map[string]any) (any, *rpcError) {
	t.Helper()
	rawArgs, _ := json.Marshal(args)
	params, _ := json.Marshal(callParams{Name: tool, Arguments: rawArgs})
	return handleToolCall(params)
}

// siteActionsUnderTest is every (tool, action) that reads a `site` argument and
// is dispatched through groupDispatch. Worktree is dispatched separately and is
// covered by TestDispatchCanonicalizesSiteArg_Worktree.
var siteActionsUnderTest = map[string][]string{
	"site": {"php", "node", "runtime", "pause", "unpause", "restart", "rebuild",
		"tls_enable", "tls_disable", "nginx_read", "nginx_write", "nginx_reset"},
	"worker": {"start", "stop", "list", "add", "remove",
		"queue_start", "queue_stop", "reverb_start", "reverb_stop",
		"horizon_start", "horizon_stop", "schedule_start", "schedule_stop",
		"stripe_start", "stripe_stop", "stripe_config"},
	"exec": {"commands_list", "commands_run", "command_add", "command_remove"},
	"logs": {"sources", "fetch"},
	"diag": {"site_doctor", "dumps_recent", "analyze_queries", "route_timing", "optimize_route"},
}

// The dispatch boundary must canonicalize a `site` argument passed as a domain to
// the internal site name before ANY action runs, so the actions that build
// systemd unit names or read paths straight from it (and used to silently no-op
// on a domain) all receive the same resolved value.
func TestDispatchCanonicalizesSiteArg(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{Name: "acme", Path: t.TempDir(), Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	for tool, actions := range siteActionsUnderTest {
		for _, action := range actions {
			orig, ok := groupDispatch[tool][action]
			if !ok {
				t.Fatalf("%s/%s is not a registered action (test list is stale)", tool, action)
			}
			var got string
			var called bool
			groupDispatch[tool][action] = func(a map[string]any) (any, *rpcError) {
				got, called = strArg(a, "site"), true
				return toolOK("ok"), nil
			}
			callToolRaw(t, tool, map[string]any{"action": action, "site": "acme.test"})
			groupDispatch[tool][action] = orig

			if !called {
				t.Errorf("%s/%s: handler was not reached", tool, action)
			} else if got != "acme" {
				t.Errorf("%s/%s: handler saw site=%q, want canonical %q", tool, action, got, "acme")
			}
		}
	}
}

// A site name must still pass through unchanged, and an unknown reference must be
// left as-is so the action produces its own "not found" rather than a rewrite.
func TestDispatchSiteArgPassthrough(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{Name: "acme", Path: t.TempDir(), Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	for _, in := range []struct{ site, want string }{{"acme", "acme"}, {"ghost.test", "ghost.test"}} {
		orig := groupDispatch["site"]["php"]
		var got string
		groupDispatch["site"]["php"] = func(a map[string]any) (any, *rpcError) {
			got = strArg(a, "site")
			return toolOK("ok"), nil
		}
		callToolRaw(t, "site", map[string]any{"action": "php", "site": in.site})
		groupDispatch["site"]["php"] = orig
		if got != in.want {
			t.Errorf("site=%q dispatched as %q, want %q", in.site, got, in.want)
		}
	}
}

// Worktree dispatches through its own switch, but canonicalization happens before
// that branch, so a domain must resolve like the name instead of erroring unknown.
func TestDispatchCanonicalizesSiteArg_Worktree(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{Name: "acme", Path: t.TempDir(), Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	res, _ := callToolRaw(t, "worktree", map[string]any{"action": "list", "site": "acme.test"})
	if mcpIsError(res) && strings.Contains(mcpText(t, res), "unknown site") {
		t.Errorf("worktree list with a domain should resolve to the site, got: %s", mcpText(t, res))
	}
}
