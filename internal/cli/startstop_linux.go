//go:build linux

package cli

import (
	"os"
	"os/exec"
	"strings"
)

// ensurePodmanMachineRunning is a no-op on Linux — Podman runs natively
// without a VM, so no machine needs to be started before running containers.
func ensurePodmanMachineRunning() error { return nil }

// migrateExecWorkerPlists is a no-op on Linux — exec-based plists only existed
// in the macOS-specific alpha.2/alpha.3 launchd plist implementation.
func migrateExecWorkerPlists() {}

// traySessionAvailable reports whether a tray started now has a display to
// open. Its unit is WantedBy graphical-session.target, so with no desktop
// logged in it is left for the next login rather than started to fail.
func traySessionAvailable() bool {
	return hasGraphicalSession(os.Getenv, func() string {
		out, _ := exec.Command("systemctl", "--user", "show-environment").Output()
		return string(out)
	})
}

func hasGraphicalSession(getenv func(string) string, managerEnv func() string) bool {
	if getenv("DISPLAY") != "" || getenv("WAYLAND_DISPLAY") != "" {
		return true
	}
	for _, line := range strings.Split(managerEnv(), "\n") {
		if strings.HasPrefix(line, "DISPLAY=") || strings.HasPrefix(line, "WAYLAND_DISPLAY=") {
			return true
		}
	}
	return false
}
