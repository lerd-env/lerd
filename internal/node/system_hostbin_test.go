package node

import (
	"path/filepath"
	"testing"
)

// withBrewPrefix points the unit PATH list at a fixture standing in for
// /opt/homebrew/bin, which a launchd daemon never has on PATH.
func withBrewPrefix(t *testing.T, dir string) {
	t.Helper()
	prev := unitPathDirs
	unitPathDirs = func() []string { return []string{dir} }
	t.Cleanup(func() { unitPathDirs = prev })
}

// A host worker unit written by lerd-ui or lerd-watcher must resolve the same
// Node the CLI does. Under launchd's PATH the Homebrew prefix is invisible, and
// falling through to an old nvm install would run the worker on a different
// Node than `lerd npm` installed its modules with.
func TestSystemNodeBinDirs_findsHomebrewUnderDaemonPATH(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")

	brew := filepath.Join(tmp, "homebrew", "bin")
	writeExec(t, brew, "node")
	writeExec(t, brew, "npm")
	withBrewPrefix(t, brew)

	// An older nvm install, the thing the daemon used to fall back to.
	stale := filepath.Join(home, ".nvm", "versions", "node", "v16.20.1", "bin")
	writeExec(t, stale, "node")
	writeExec(t, stale, "npm")

	t.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")

	dirs := SystemNodeBinDirs()
	if len(dirs) != 1 || dirs[0] != brew {
		t.Fatalf("want [%s], got %v", brew, dirs)
	}
}

// With no Homebrew node, the version-manager fallback still applies.
func TestSystemNodeBinDirs_stillFallsBackToNvm(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")

	withBrewPrefix(t, filepath.Join(tmp, "empty-prefix"))

	nvmBin := filepath.Join(home, ".nvm", "versions", "node", "v22.1.0", "bin")
	writeExec(t, nvmBin, "node")
	writeExec(t, nvmBin, "npm")

	t.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")

	dirs := SystemNodeBinDirs()
	if len(dirs) != 1 || dirs[0] != nvmBin {
		t.Fatalf("want [%s], got %v", nvmBin, dirs)
	}
}

// A real PATH entry still wins over the extra prefixes, so a user who puts a
// specific Node first in their shell keeps it.
func TestSystemNodeBinDirs_pathWinsOverExtraDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("NVM_DIR", "")

	onPath := filepath.Join(tmp, "chosen")
	writeExec(t, onPath, "node")
	writeExec(t, onPath, "npm")
	brew := filepath.Join(tmp, "homebrew", "bin")
	writeExec(t, brew, "node")
	writeExec(t, brew, "npm")
	withBrewPrefix(t, brew)

	t.Setenv("PATH", onPath)

	dirs := SystemNodeBinDirs()
	if len(dirs) != 1 || dirs[0] != onPath {
		t.Fatalf("want [%s], got %v", onPath, dirs)
	}
}
