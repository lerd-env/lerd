//go:build windows

package unitlog

import (
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
)

// LogPath is where a lerd-supervised host unit writes its log on Windows.
func LogPath(unit string) string {
	return filepath.Join(config.DataDir(), "logs", unit+".log")
}

// LogHint is the command a user runs to read a unit's recent output, PowerShell
// here since neither journalctl nor tail exist.
func LogHint(unit string) string {
	return "Get-Content -Tail 20 '" + LogPath(unit) + "'"
}

// IsContainerUnit mirrors the macOS rule: the daemons and any unit with an
// on-disk worker guard script run as host processes, framework workers follow
// the configured exec mode, everything else is a detached podman container.
func IsContainerUnit(unit string) bool {
	switch unit {
	case "lerd-dns", "lerd-watcher", "lerd-ui":
		return false
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
