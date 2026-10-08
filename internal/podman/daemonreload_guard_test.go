package podman

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A suite run used to fire dozens of real daemon-reloads at the developer's own
// user manager. The child gets a fresh bus connection pointed nowhere, so any
// reload that is attempted falls through to the fake systemctl and leaves a mark.
func TestDaemonReloadNeverReachesTheRealUserManager(t *testing.T) {
	if os.Getenv("LERD_DAEMONRELOAD_CHILD") == "1" {
		_ = DaemonReload()
		return
	}

	bin := t.TempDir()
	mark := filepath.Join(t.TempDir(), "reloaded")
	script := "#!/bin/sh\ntouch " + mark + "\n"
	if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestDaemonReloadNeverReachesTheRealUserManager$")
	cmd.Env = append(os.Environ(),
		"LERD_DAEMONRELOAD_CHILD=1",
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DBUS_SESSION_BUS_ADDRESS=unix:path="+filepath.Join(bin, "no-bus"),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	if _, err := os.Stat(mark); err == nil {
		t.Fatal("DaemonReload reached systemd from a test binary")
	}
}
