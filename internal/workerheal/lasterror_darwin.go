//go:build darwin

package workerheal

import (
	"os"
	"path/filepath"
)

// readLastErrorPlatform tails the launchd log file for a unit. The lerd
// service-manager redirects stdout+stderr from each plist to
// ~/Library/Logs/lerd/<unit>.log (see services/launchd_darwin.go), so this
// is the macOS analogue of `journalctl -u <unit> -n 1`. Returns "" when the
// file doesn't exist or contains no usable lines.
func readLastErrorPlatform(unit string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, "Library", "Logs", "lerd", unit+".log")
	return lastNonBlankLine(path)
}
