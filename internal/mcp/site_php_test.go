package mcp

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// execSitePHP must reject an unsupported version before it writes .php-version
// or touches the registry, so a typo like "9.9" returns a clean error instead
// of persisting a bad pin that only fails later with "container not found".
func TestExecSitePHP_RejectsUnsupportedVersion(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{Name: "acme", Path: t.TempDir(), Domains: []string{"acme.test"}, PHPVersion: "8.4"}); err != nil {
		t.Fatal(err)
	}

	res, rpcErr := execSitePHP(map[string]any{"site": "acme", "version": "9.9"})
	if rpcErr != nil {
		t.Fatalf("unexpected rpcError: %v", rpcErr)
	}
	if !mcpIsError(res) {
		t.Fatalf("expected an error result for an unsupported version, got %v", res)
	}
	if msg := mcpText(t, res); !strings.Contains(msg, "unsupported PHP version") {
		t.Errorf("error message = %q, want it to name the unsupported version", msg)
	}

	// The registry must be untouched — no bad version persisted.
	site, err := config.FindSite("acme")
	if err != nil {
		t.Fatal(err)
	}
	if site.PHPVersion != "8.4" {
		t.Errorf("site PHPVersion = %q, want 8.4 (unchanged)", site.PHPVersion)
	}
}
