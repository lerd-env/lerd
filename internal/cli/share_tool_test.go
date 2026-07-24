package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestShareToolNames_matchBinaries(t *testing.T) {
	names := shareToolNames()
	if len(names) != len(shareTools) {
		t.Fatalf("shareToolNames() returned %d names for %d tools", len(names), len(shareTools))
	}
	for _, name := range names {
		bin, ok := shareToolBinary(name)
		if !ok {
			t.Errorf("%q is offered but has no binary mapping", name)
		}
		if bin == "" {
			t.Errorf("%q maps to an empty binary", name)
		}
	}
	if _, ok := shareToolBinary("auto"); ok {
		t.Error(`"auto" must not resolve to a binary, it clears the default`)
	}
	if _, ok := shareToolBinary("bogus"); ok {
		t.Error("an unknown tool must not resolve to a binary")
	}
}

// The order is what the command line and the "change it with" hint offer, and it
// mirrors the auto-detection order in pickShareTool.
func TestShareToolNames_order(t *testing.T) {
	want := "ngrok|cloudflare|expose|serveo|localhost-run"
	if got := strings.Join(shareToolNames(), "|"); got != want {
		t.Errorf("shareToolNames() = %q, want %q", got, want)
	}
}

func TestNewShareToolCmd_helpMentionsEveryTool(t *testing.T) {
	cmd := NewShareToolCmd()
	help := cmd.Use + "\n" + cmd.Long + "\n" + cmd.Example
	for _, name := range shareToolNames() {
		if !strings.Contains(help, name) {
			t.Errorf("help does not mention %q:\n%s", name, help)
		}
	}
	for _, want := range []string{"auto", "lerd share:tool cloudflare"} {
		if !strings.Contains(help, want) {
			t.Errorf("help does not mention %q:\n%s", want, help)
		}
	}
}

func TestRunShareTool_rejectsUnknownTool(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	err := runShareTool(nil, []string{"bogus"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), `unknown tool "bogus"`) {
		t.Errorf("error = %v, want it to name the rejected tool", err)
	}
	for _, name := range shareToolNames() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error should list %q as a valid choice, got %v", name, err)
		}
	}
}

func TestRunShareTool_rejectsToolWithoutItsBinary(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	err := runShareTool(nil, []string{"ngrok"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "not in PATH") {
		t.Errorf("error = %v, want it to say the binary is missing", err)
	}
}

func TestRunShareTool_setShowReset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PATH", fakeBin(t, "ngrok"))

	if err := runShareTool(nil, []string{"ngrok"}); err != nil {
		t.Fatalf("setting ngrok: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Share.DefaultTool != "ngrok" {
		t.Errorf("DefaultTool = %q, want ngrok", cfg.Share.DefaultTool)
	}

	// Showing the default also has to say how to change it, otherwise the command
	// reports a value with no way to act on it.
	out := captureStdout(t, func() {
		if err := runShareTool(nil, nil); err != nil {
			t.Errorf("showing: %v", err)
		}
	})
	if !strings.Contains(out, "ngrok") {
		t.Errorf("show output %q does not report the current tool", out)
	}
	if !strings.Contains(out, "lerd share:tool") {
		t.Errorf("show output %q does not say how to change it", out)
	}

	if err := runShareTool(nil, []string{"auto"}); err != nil {
		t.Fatalf("resetting: %v", err)
	}
	cfg, err = config.LoadGlobal()
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	if cfg.Share.DefaultTool != "" {
		t.Errorf("DefaultTool = %q after auto, want it cleared", cfg.Share.DefaultTool)
	}
}
