package mcp

import (
	"strings"
	"testing"
)

func TestExtractSpxReport(t *testing.T) {
	// The command's own stdout precedes the report; only the report is returned.
	combined := "done\nsome app output\n\n*** SPX Report ***\n\nFlat profile:\n work 5.1ms\n"
	got := extractSpxReport(combined)
	if strings.HasPrefix(got, "done") || strings.Contains(got, "app output") {
		t.Errorf("command stdout was not dropped: %q", got)
	}
	if !strings.HasPrefix(got, "*** SPX Report ***") {
		t.Errorf("report should start at the SPX marker, got: %q", got)
	}

	// No marker: return the whole trimmed output so a caller can tell SPX did not run.
	noMarker := "  Fatal error: something broke  "
	if got := extractSpxReport(noMarker); got != "Fatal error: something broke" {
		t.Errorf("without a marker, got %q", got)
	}
}

func TestExecProfilerReport_RequiresArgs(t *testing.T) {
	// A resolvable path but no argv must fail before any container work.
	res, rpcErr := execProfilerReport(map[string]any{"path": "/tmp/some-project"})
	if rpcErr != nil {
		t.Fatalf("unexpected rpcError: %v", rpcErr)
	}
	if !mcpIsError(res) {
		t.Fatal("expected an error result when args is missing")
	}
	if msg := mcpText(t, res); !strings.Contains(msg, "args is required") {
		t.Errorf("error should name the missing args, got: %q", msg)
	}
}

func TestExecProfilerReport_UnknownSite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	res, rpcErr := execProfilerReport(map[string]any{"site": "ghost", "args": []any{"artisan", "x"}})
	if rpcErr != nil {
		t.Fatalf("unexpected rpcError: %v", rpcErr)
	}
	if !mcpIsError(res) || !strings.Contains(mcpText(t, res), "site not found") {
		t.Errorf("expected a site-not-found error, got: %v", res)
	}
}
