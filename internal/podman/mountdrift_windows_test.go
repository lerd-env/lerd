//go:build windows

package podman

import (
	"os"
	"path/filepath"
	"testing"
)

// A project under the user profile is reachable through the %h mount every lerd
// container has, so asking for it must neither touch a quadlet nor restart.
func TestEnsurePathMountedLeavesAPathUnderTheProfileAlone(t *testing.T) {
	home := t.TempDir()
	cfgHome := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	resetPathMountAttempts()

	quadlets := filepath.Join(cfgHome, "containers", "systemd")
	if err := os.MkdirAll(quadlets, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "[Container]\nVolume=%h:%h:rw\n"
	fpm := filepath.Join(quadlets, "lerd-php84-fpm.container")
	if err := os.WriteFile(fpm, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	lc := &restartRecorder{}
	prevLC := UnitLifecycle
	UnitLifecycle = lc
	t.Cleanup(func() { UnitLifecycle = prevLC })

	EnsurePathMounted(filepath.Join(home, "projects", "app"), "8.4")

	if len(lc.restarted) != 0 {
		t.Errorf("restarted %v for a path the home mount already covers", lc.restarted)
	}
	if got, _ := os.ReadFile(fpm); string(got) != content {
		t.Errorf("quadlet rewritten:\n%s", got)
	}
}
