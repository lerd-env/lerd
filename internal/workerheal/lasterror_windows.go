//go:build windows

package workerheal

import (
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
)

// readLastErrorPlatform tails the log file the Windows service manager
// redirects each unit's output to, the analogue of journalctl -u <unit> -n 1.
func readLastErrorPlatform(unit string) string {
	return lastNonBlankLine(filepath.Join(config.DataDir(), "logs", unit+".log"))
}
