//go:build darwin

package unitlog

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// LogPath is where a launchd-supervised lerd unit writes its log on macOS.
func LogPath(unit string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Logs", "lerd", unit+".log")
}

// LogHint is the command a user runs to read a unit's recent output. It lives
// here rather than beside each caller so a message that offers it cannot end up
// naming journalctl, which no Mac has.
func LogHint(unit string) string {
	return "tail -n 20 " + LogPath(unit)
}

// IsContainerUnit returns true for units that run as detached podman containers
// (podman run -d). Their logs come from `podman logs`, not the launchd log file.
//
// In exec mode, framework workers run as launchd service units; host workers
// (vite + future Node tooling) always do, regardless of mode. Detection reads
// the plist when present (RunAtLoad = service unit), and the on-disk guard
// script as a second tell — host workers always write one, container workers
// never do. Falls back to the global config for known framework worker patterns
// when neither artifact is on disk.
func IsContainerUnit(unit string) bool {
	switch unit {
	case "lerd-dns", "lerd-watcher", "lerd-ui":
		return false
	}
	home, _ := os.UserHomeDir()
	if plist, err := os.ReadFile(filepath.Join(home, "Library", "LaunchAgents", unit+".plist")); err == nil {
		return !strings.Contains(string(plist), "<key>RunAtLoad</key>")
	}
	if _, err := os.Stat(filepath.Join(config.RunDir(), "workers", unit+".sh")); err == nil {
		return false
	}
	if IsFrameworkWorkerUnit(unit) {
		cfg, _ := config.LoadGlobal()
		if cfg != nil && cfg.WorkerExecMode() != config.WorkerExecModeContainer {
			return false
		}
	}
	return true
}
