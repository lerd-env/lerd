package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/hostbin"
)

// lerd-ui runs under launchd with PATH=/usr/bin:/bin:/usr/sbin:/sbin, so a
// cloudflared installed by Homebrew is not on PATH for the dashboard even
// though `lerd share` finds it in a terminal.
func withBrewOnlyTunnelTools(t *testing.T, names ...string) string {
	t.Helper()
	brew := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(brew, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	prev := hostbin.ExtraDirs
	hostbin.ExtraDirs = func() []string { return []string{brew} }
	t.Cleanup(func() { hostbin.ExtraDirs = prev })
	t.Setenv("PATH", t.TempDir())
	return brew
}

func TestShareToolsReportsBrewInstalledTools(t *testing.T) {
	withBrewOnlyTunnelTools(t, "cloudflared", "ngrok")

	info := ShareTools()
	got := map[string]bool{}
	for _, tool := range info.Tools {
		got[tool.Name] = tool.Installed
	}
	if !got["cloudflare"] {
		t.Error("cloudflared installed under a Homebrew prefix reported as not installed")
	}
	if !got["ngrok"] {
		t.Error("ngrok installed under a Homebrew prefix reported as not installed")
	}
	if got["expose"] {
		t.Error("expose reported installed when it is nowhere")
	}
}

func TestAutoShareToolPicksBrewInstalledTool(t *testing.T) {
	withBrewOnlyTunnelTools(t, "cloudflared")

	if got := autoShareToolName("", ""); got != "cloudflare" {
		t.Fatalf("autoShareToolName() = %q; want cloudflare", got)
	}
}

func TestPickShareToolAcceptsBrewInstalledTool(t *testing.T) {
	withBrewOnlyTunnelTools(t, "cloudflared")

	tool, err := pickShareTool(false, true, false, false, false, false, "", "", "", "")
	if err != nil {
		t.Fatalf("pickShareTool() = %v; want the Homebrew cloudflared accepted", err)
	}
	if tool.mode != shareModeCloudflare {
		t.Fatalf("mode = %v; want cloudflare", tool.mode)
	}
}

// The command must name the resolved binary, not the bare name: the daemon's
// PATH cannot find it at exec time either.
func TestTunnelCommandUsesResolvedBinary(t *testing.T) {
	brew := withBrewOnlyTunnelTools(t, "cloudflared")

	cmd, stop, err := buildTunnelCommand(&shareTool{mode: shareModeCloudflare}, "",
		shareTarget{name: "demo", domain: "demo.test"}, 80, 443, true)
	if err != nil {
		t.Fatalf("buildTunnelCommand() = %v", err)
	}
	defer stop()
	want := filepath.Join(brew, "cloudflared")
	if cmd.Path != want && !strings.HasPrefix(cmd.Args[0], brew) {
		t.Fatalf("command = %q %v; want it to run %s", cmd.Path, cmd.Args, want)
	}
}
