package serviceops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// A service put to sleep has to stay down through a reboot: its quadlet loses
// [Install] while it sleeps and gets it back when it wakes.
func TestSetIdleSuspended_togglesBootStart(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	reloads := 0
	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { reloads++; return nil }

	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const withInstall = "[Container]\nImage=x\n\n[Install]\nWantedBy=default.target\n"
	for _, n := range []string{"mysql", "phpmyadmin"} {
		if err := os.WriteFile(filepath.Join(dir, "lerd-"+n+".container"), []byte(withInstall), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hasInstall := func(n string) bool {
		b, _ := os.ReadFile(filepath.Join(dir, "lerd-"+n+".container"))
		return strings.Contains(string(b), "[Install]")
	}

	SetIdleSuspended([]string{"mysql", "phpmyadmin"}, true)
	for _, n := range []string{"mysql", "phpmyadmin"} {
		if !config.ServiceIsIdleSuspended(n) {
			t.Errorf("%s not flagged asleep", n)
		}
		if hasInstall(n) {
			t.Errorf("%s still starts at boot while asleep", n)
		}
	}
	if reloads != 1 {
		t.Errorf("reloads = %d, want 1 for the batch", reloads)
	}

	SetIdleSuspended([]string{"mysql", "phpmyadmin"}, false)
	for _, n := range []string{"mysql", "phpmyadmin"} {
		if config.ServiceIsIdleSuspended(n) {
			t.Errorf("%s still flagged asleep", n)
		}
		if !hasInstall(n) {
			t.Errorf("%s does not start at boot after waking", n)
		}
	}
	if reloads != 2 {
		t.Errorf("reloads = %d, want 2", reloads)
	}
}
