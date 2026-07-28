package systemd

import (
	"strings"
	"testing"
)

// Homebrew installs the binary into a version-pinned Cellar directory and puts
// a stable symlink on PATH. Resolving the symlink pins the unit to the version
// that was current when it was written, so the next `brew upgrade lerd` deletes
// the path the plist points at and the daemons stop starting at login.
func TestGetUnitKeepsStableBrewPath(t *testing.T) {
	for _, brewLink := range []string{
		"/opt/homebrew/bin/lerd",
		"/usr/local/bin/lerd",
	} {
		prev := lerdBinaryPath
		lerdBinaryPath = func() string { return brewLink }
		unit, err := GetUnit("lerd-ui")
		lerdBinaryPath = prev
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(unit, "ExecStart="+brewLink+" serve-ui") {
			t.Errorf("unit does not run %s:\n%s", brewLink, unit)
		}
	}
}

func TestResolveBinaryPathKeepsCellarSymlink(t *testing.T) {
	link := "/opt/homebrew/bin/lerd"
	target := "/opt/homebrew/Cellar/lerd/1.31.0/bin/lerd"

	if got := stableBinaryPath(link, target); got != link {
		t.Errorf("stableBinaryPath() = %q; want the symlink %q, which survives a brew upgrade", got, link)
	}
}

// Every other install resolves symlinks as before, so an ostree /usr/local or a
// ~/.local/bin symlink still points at the real binary.
func TestResolveBinaryPathResolvesOrdinarySymlink(t *testing.T) {
	link := "/home/u/.local/bin/lerd"
	target := "/home/u/apps/lerd-1.31.0/lerd"

	if got := stableBinaryPath(link, target); got != target {
		t.Errorf("stableBinaryPath() = %q; want the resolved %q", got, target)
	}
}
