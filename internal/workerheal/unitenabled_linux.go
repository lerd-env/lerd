package workerheal

import (
	"os"
	"path/filepath"
)

// isUnitEnabled reports whether a user systemd unit is enabled, i.e. supposed
// to be running. `systemctl --user enable` creates a wants-symlink under
// default.target.wants/ and `disable` removes it; `lerd worker stop` disables,
// so a present symlink means the unit is meant to be up. A pure filesystem
// stat — no subprocess — so it's safe to call per candidate from the detector.
func isUnitEnabled(unit string) bool {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return false
	}
	link := filepath.Join(dir, "systemd", "user", "default.target.wants", unit)
	if _, err := os.Lstat(link); err == nil {
		return true
	}
	return false
}
