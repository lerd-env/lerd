package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/services"
	lerdSystemd "github.com/geodro/lerd/internal/systemd"
)

// daemonUnits are the lerd services whose ExecStart names the lerd binary, so
// they are the ones a moved binary takes down. lerd-autostart is macOS-only and
// reads as absent elsewhere.
var daemonUnits = []string{"lerd-ui", "lerd-watcher", "lerd-tray", "lerd-autostart", dnsUnit}

// healLerdBinaryMove repairs what a lerd binary that moved leaves behind, and
// reports what it rewrote. A package manager can replace lerd underneath a
// working install: `brew upgrade lerd` retires the previous version's keg, and
// every unit and shim written against it then runs a path that is gone, which
// surfaces as daemons failing with a bare exit status 203 and `php` reporting
// no such file (#1432).
func healLerdBinaryMove() (units, shims []string) {
	return healLerdBinaryPaths(binaryGone)
}

// healLerdBinaryUpgrade is the same repair from the package-manager hook, which
// runs from the install that has just landed. It also repoints an older keg of
// the same formula while brew still has it on disk: the cleanup that deletes it
// comes later, and by then there is nobody at the terminal to notice.
func healLerdBinaryUpgrade() (units, shims []string) {
	return healLerdBinaryPaths(binarySuperseded)
}

func healLerdBinaryPaths(superseded func(string) bool) (units, shims []string) {
	return healDaemonUnits(superseded), healShimBinaryPaths(config.LerdBinary(), superseded)
}

// binaryGone is the rule the start repairs by: only a path that is not there is
// rewritten, so starting a build from a checkout cannot take the login daemons
// of a working install with it.
func binaryGone(path string) bool {
	_, err := os.Stat(path)
	return err != nil
}

func binarySuperseded(path string) bool {
	return config.SupersededBinary(path, config.LerdBinary())
}

// repairSummary is the line the start prints once a move has been repaired, so
// a rewrite of the user's own files is never silent.
func repairSummary(units, shims []string) string {
	var parts []string
	if len(units) > 0 {
		parts = append(parts, "services "+strings.Join(units, ", "))
	}
	if len(shims) > 0 {
		parts = append(parts, "shims "+strings.Join(shims, ", "))
	}
	return "Repointed " + strings.Join(parts, " and ") + " at " + config.LerdBinary()
}

// healDaemonUnits rewrites daemon units whose ExecStart names a binary the
// caller's rule counts as superseded, so they run the lerd that is installed
// now. A unit the rule leaves alone is left exactly as installed.
func healDaemonUnits(superseded func(string) bool) []string {
	var healed []string
	for _, name := range daemonUnits {
		installed := services.InstalledUnitBinary(name)
		if installed == "" || !superseded(installed) {
			continue
		}
		content, err := daemonUnitContent(name)
		if err != nil {
			continue
		}
		// Nothing to gain when the rewrite would name the same missing file,
		// which is what a host without the tray helper installed looks like.
		if services.UnitExecBinary(content) == installed {
			continue
		}
		if err := writeUserServiceWithReload(name, content); err != nil {
			fmt.Printf("  WARN: repointing the %s unit at %s: %v\n", name, config.LerdBinary(), err)
			continue
		}
		healed = append(healed, name)
	}
	return healed
}

// healShimBinaryPaths repoints shims in lerd's bin dir that run a lerd binary
// the caller's rule counts as superseded. Everything else is left exactly as
// installed, so starting a build from a checkout never takes the user's shims
// with it.
func healShimBinaryPaths(lerdBin string, superseded func(string) bool) []string {
	if info, err := os.Stat(lerdBin); err != nil || info.IsDir() {
		return nil
	}
	binDir := config.BinDir()
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return nil
	}
	var healed []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(binDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil || !strings.HasPrefix(string(data), "#!") {
			continue
		}
		repaired, changed := healedShim(string(data), lerdBin, superseded)
		if !changed {
			continue
		}
		if err := os.WriteFile(path, []byte(repaired), 0755); err != nil {
			fmt.Printf("  WARN: repointing the %s shim at %s: %v\n", entry.Name(), lerdBin, err)
			continue
		}
		healed = append(healed, entry.Name())
	}
	return healed
}

// healedShim swaps every absolute path to a superseded lerd binary for lerdBin.
// Anything else the shim runs (composer.phar, a version manager, the tool
// itself) is left alone.
func healedShim(content, lerdBin string, superseded func(string) bool) (string, bool) {
	changed := false
	for _, token := range shimTokens(content) {
		if token == lerdBin || !filepath.IsAbs(token) || filepath.Base(token) != "lerd" {
			continue
		}
		if !superseded(token) {
			continue
		}
		content = strings.ReplaceAll(content, token, lerdBin)
		changed = true
	}
	return content, changed
}

// shimTokens splits shim source into the words a shell would see, so a path is
// recognised whether it was written bare or quoted.
func shimTokens(content string) []string {
	return strings.FieldsFunc(content, func(r rune) bool {
		return unicode.IsSpace(r) || r == '"' || r == '\''
	})
}

// daemonUnitContent is the unit a daemon is rewritten with. lerd-dns is built
// in code rather than embedded, since its listen address depends on the platform.
func daemonUnitContent(name string) (string, error) {
	if name == dnsUnit {
		return dnsServiceContent(config.LerdBinary(), dnsListenHost()), nil
	}
	return lerdSystemd.GetUnit(name)
}
