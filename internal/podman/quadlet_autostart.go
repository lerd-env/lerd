package podman

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// QuadletInstallBlock is the [Install] stanza every lerd-*.container quadlet
// ships with, so a strip-then-restore round-trip yields the same file.
const QuadletInstallBlock = "[Install]\nWantedBy=default.target\n"

// SetQuadletAutostart strips (on=false) or restores (on=true) the [Install]
// section of an installed quadlet, so the unit stops or resumes starting at
// boot. Restoring never re-arms a unit while global autostart is off. Reports
// whether the file changed; the caller daemon-reloads when it did.
func SetQuadletAutostart(unit string, on bool) bool {
	path := filepath.Join(config.QuadletDir(), unit+".container")
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	out := StripInstallSection(string(raw), true)
	if on && !globalAutostartDisabled() {
		out = strings.TrimRight(out, "\n") + "\n\n" + QuadletInstallBlock
	}
	if out == string(raw) {
		return false
	}
	config.GuardRealWrite(path)
	return os.WriteFile(path, []byte(out), 0644) == nil
}

func globalAutostartDisabled() bool {
	cfg, err := config.LoadGlobal()
	return err == nil && cfg != nil && cfg.Autostart.Disabled
}
