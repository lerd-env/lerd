package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A quadlet is written while a path is there and keeps naming it after it goes.
// Podman then refuses the container instead of skipping the mount, so the unit
// restart-loops and every site on that version answers 502, which is what a
// registered ODBC driver does when the vendor client is uninstalled.
func TestStaleQuadletMountsFindsAPathThatWentAway(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	if err := os.MkdirAll(config.QuadletDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	present := t.TempDir()
	missing := filepath.Join(tmp, "vendor-client-that-was-removed")

	unit := "[Container]\nImage=x\n" +
		"Volume=%h:%h:rw\n" +
		"Volume=lerd-ssh-agent:/ssh-agent\n" +
		"Volume=" + present + ":" + present + ":ro\n" +
		"Volume=" + missing + ":" + missing + ":ro\n"
	if err := os.WriteFile(filepath.Join(config.QuadletDir(), SharedFPMContainerName("8.4")+".container"), []byte(unit), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := StaleQuadletMounts()
	if len(stale) != 1 || stale[0] != missing {
		t.Fatalf("StaleQuadletMounts() = %v, want only %q", stale, missing)
	}
	for _, s := range stale {
		if strings.HasPrefix(s, "%h") || s == "lerd-ssh-agent" {
			t.Errorf("a named volume or specifier was treated as a host path: %q", s)
		}
	}
}
