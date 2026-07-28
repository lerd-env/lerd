package systemd

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed units
var unitsFS embed.FS

// lerdBinaryPath resolves the absolute path to the running lerd binary so unit
// ExecStart lines point at wherever lerd is actually installed: ~/.local/bin
// for curl/brew, /usr/bin for the deb. A var so tests can override it; returns
// "" when the path can't be resolved, in which case GetUnit leaves the template
// default in place.
var lerdBinaryPath = func() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, rErr := filepath.EvalSymlinks(exe); rErr == nil {
		return stableBinaryPath(exe, resolved)
	}
	return exe
}

// stableBinaryPath picks which spelling of the binary a unit should carry.
// Normally the resolved one, so a symlink that moves cannot break the unit. A
// Homebrew Cellar is the exception: it is version-pinned, so resolving there
// writes a path that `brew upgrade lerd` deletes, leaving the daemons pointing
// at a binary that no longer exists. Homebrew's own symlink is the stable name,
// so that one is kept.
func stableBinaryPath(exe, resolved string) string {
	if strings.Contains(resolved, "/Cellar/") {
		return exe
	}
	return resolved
}

// GetUnit returns the content of an embedded systemd unit file with the lerd
// binary path resolved. The templates ship with ExecStart=%h/.local/bin/lerd,
// which only works for a ~/.local/bin install; substituting the real path lets
// the daemon units run from any install location (notably /usr/bin under the
// Debian package).
func GetUnit(name string) (string, error) {
	// name may or may not have .service suffix
	filename := name
	if !strings.HasSuffix(filename, ".service") {
		filename += ".service"
	}
	data, err := unitsFS.ReadFile("units/" + filename)
	if err != nil {
		return "", fmt.Errorf("systemd unit %q not found: %w", name, err)
	}
	return resolveUnitBinaryPath(string(data)), nil
}

// resolveUnitBinaryPath swaps the hardcoded ~/.local/bin templates for the
// running binary's real location. lerd-tray is replaced first because its name
// has lerd as a prefix.
func resolveUnitBinaryPath(content string) string {
	bin := lerdBinaryPath()
	if bin == "" {
		return content
	}
	tray := filepath.Join(filepath.Dir(bin), "lerd-tray")
	content = strings.ReplaceAll(content, "%h/.local/bin/lerd-tray", tray)
	content = strings.ReplaceAll(content, "%h/.local/bin/lerd", bin)
	return content
}
