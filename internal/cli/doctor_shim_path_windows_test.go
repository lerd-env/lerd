//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// On Windows the shim a shell runs is node.cmd, so that is what lerd's own
// shim has to be compared as, or doctor reports lerd shadowing itself.
func TestResolvedShimPathMatchesTheCmdShim(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	bin := config.BinDir()
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"node", "node.cmd"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)

	shim, resolved, err := resolvedShimPath("node")
	if err != nil {
		t.Fatal(err)
	}
	if status, detail := shimShadowFinding("node", shim, resolved, nil); status != "ok" {
		t.Errorf("lerd's own node.cmd reported as a shadow: shim %q, resolved %q, %s", shim, resolved, detail)
	}
}
