//go:build linux

package workerheal

import (
	"os/exec"
	"strings"
)

// readLastErrorPlatform asks journalctl for the most recent log line of a
// failed worker. Returns "" if journalctl is missing or the unit has no
// entries — the caller treats that as "no excerpt available".
func readLastErrorPlatform(unit string) string {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return ""
	}
	cmd := exec.Command("journalctl", "--user", "-u", unit, "-n", "1", "--no-pager", "-o", "cat")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
