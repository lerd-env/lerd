package serviceops

import (
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/podman"
)

// `lerd service preset` and the auto-install inside `lerd link` install through
// here rather than the streaming path the dashboard uses, and a service
// installed from the CLI needs its dashboard served just as much.
func TestInstallPresetByName_SyncsTheDashboardVhost(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }

	synced := 0
	prev := syncLerdVhostFn
	syncLerdVhostFn = func() (bool, error) { synced++; return false, nil }
	t.Cleanup(func() { syncLerdVhostFn = prev })

	if _, err := InstallPresetByName("redis", ""); err != nil {
		t.Fatalf("InstallPresetByName: %v", err)
	}
	if synced != 1 {
		t.Errorf("vhost synced %d times, want 1 after a CLI install", synced)
	}
}
