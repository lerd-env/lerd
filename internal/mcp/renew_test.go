package mcp

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestExecRenew_requiresSite(t *testing.T) {
	res, _ := execRenew(map[string]any{})
	if !mcpIsError(res) || !strings.Contains(mcpText(t, res), "site is required") {
		t.Errorf("expected 'site is required', got: %s", mcpText(t, res))
	}
}

func TestExecRenew_unknownSite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	res, _ := execRenew(map[string]any{"site": "ghost"})
	if !mcpIsError(res) || !strings.Contains(mcpText(t, res), "not found") {
		t.Errorf("expected not-found error, got: %s", mcpText(t, res))
	}
}

// Renewing a site that isn't secured has no certificate to reissue, so it must
// fail cleanly before touching mkcert rather than reissuing a cert for an HTTP
// site.
func TestExecRenew_refusesUnsecuredSite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{Name: "acme", Path: t.TempDir(), Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	res, _ := execRenew(map[string]any{"site": "acme"})
	if !mcpIsError(res) || !strings.Contains(mcpText(t, res), "not secured") {
		t.Errorf("expected not-secured error, got: %s", mcpText(t, res))
	}
}
