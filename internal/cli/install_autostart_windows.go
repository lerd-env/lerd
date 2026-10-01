//go:build windows

package cli

import (
	"os"

	"golang.org/x/sys/windows/registry"

	"github.com/geodro/lerd/internal/feedback"
)

const (
	runKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "lerd-autostart"
)

// autostartCommand is the Run-key value: quoted so an exe path with spaces
// survives, followed by the subcommand that brings the environment up.
func autostartCommand(exe string) string {
	return `"` + exe + `" start`
}

// installAutostart registers `lerd start` under the current user's Run key so
// lerd comes up at every login, on by default as on macOS. Nothing needs
// elevation: HKCU is the user's own hive.
func installAutostart() {
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
