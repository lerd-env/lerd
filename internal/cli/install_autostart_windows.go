//go:build windows

package cli

import (
	"os"

	"golang.org/x/sys/windows/registry"

	"github.com/geodro/lerd/internal/feedback"
	lerdSystemd "github.com/geodro/lerd/internal/systemd"
)

// runKeyPath is a variable so tests can point it at a throwaway key.
var runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

const runValueName = "lerd-autostart"

// autostartCommand is the Run-key value: quoted so an exe path with spaces
// survives, followed by the subcommand that brings the environment up.
func autostartCommand(exe string) string {
	return `"` + exe + `" start`
}

// installAutostart registers `lerd start` under the current user's Run key so
// lerd comes up at every login, on by default as on macOS. Nothing needs
// elevation: HKCU is the user's own hive.
func installAutostart() {
	if !lerdSystemd.IsAutostartEnabled() {
		_ = removeAutostart()
		return
	}
	exe, err := os.Executable()
	if err != nil {
		feedback.WarnOn(os.Stderr, "autostart: %v", err)
		return
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		feedback.WarnOn(os.Stderr, "opening the Run key: %v", err)
		return
	}
	defer k.Close()
	if err := k.SetStringValue(runValueName, autostartCommand(exe)); err != nil {
		feedback.WarnOn(os.Stderr, "enabling autostart: %v", err)
	}
}

// removeAutostart drops the Run-key entry; a missing one is not an error.
func removeAutostart() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	if err := k.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

// syncLoginAutostart makes the Run entry follow the autostart preference: a
// Windows login starts lerd from the registry, not from a service unit.
func syncLoginAutostart(disabled bool) {
	if disabled {
		if err := removeAutostart(); err != nil {
			feedback.WarnOn(os.Stderr, "disabling autostart: %v", err)
		}
		return
	}
	installAutostart()
}
