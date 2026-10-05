// Package hostbin resolves host binaries that live outside the PATH a daemon
// inherits. lerd-ui and lerd-watcher are started by launchd (or systemd), which
// hands them a minimal PATH — on macOS literally /usr/bin:/bin:/usr/sbin:/sbin —
// so a tool the user installed with Homebrew is invisible to exec.LookPath even
// though it works fine in their terminal. Anything a daemon may have to run goes
// through here so the CLI and the daemons resolve the same binary.
package hostbin

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
)

// ExtraDirs lists the install prefixes a daemon's PATH leaves out: lerd's own
// bin dir, both Homebrew prefixes on macOS (Apple Silicon and Intel), and the
// equivalents on Linux. Ordered the way a login shell would see them. A var so tests can point
// it at a fixture instead of depending on what the host has installed.
var ExtraDirs = func() []string {
	// lerd's own bin dir comes first: a tool dropped in there is on the user's
	// PATH only through `lerd path:enable`, which a daemon never inherits.
	return append([]string{config.BinDir()}, osDirs...)
}

// Look resolves name to an absolute path: PATH first, then the extra dirs.
// Reports false when the binary is nowhere to be found.
func Look(name string) (string, bool) {
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	for _, dir := range ExtraDirs() {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return p, true
		}
	}
	return "", false
}

// Path is Look for callers building a command line: it returns the resolved
// absolute path, or the bare name so the failure surfaces as the OS's own
// "executable file not found" rather than as a silent substitution.
func Path(name string) string {
	if p, ok := Look(name); ok {
		return p
	}
	return name
}
