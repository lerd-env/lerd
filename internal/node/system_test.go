package node

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// writeExec drops an executable stub named bin into dir, creating dir first.
func writeExec(t *testing.T, dir, bin string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, bin), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestSystemNodeBinDirs_findsNodeAndNpmOnPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("HOME", filepath.Join(tmp, "home"))

	binDir := filepath.Join(tmp, "nodebin")
	writeExec(t, binDir, "node")
	writeExec(t, binDir, "npm")
	t.Setenv("PATH", binDir)

	dirs := SystemNodeBinDirs()
	if len(dirs) != 1 || dirs[0] != binDir {
		t.Errorf("want [%s], got %v", binDir, dirs)
	}
}

func TestSystemNodeBinDirs_nodeAndNpmInDifferentPathDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("HOME", filepath.Join(tmp, "home"))

	nodeDir := filepath.Join(tmp, "a")
	npmDir := filepath.Join(tmp, "b")
	writeExec(t, nodeDir, "node")
	writeExec(t, npmDir, "npm")
	t.Setenv("PATH", nodeDir+string(os.PathListSeparator)+npmDir)

	dirs := SystemNodeBinDirs()
	if len(dirs) != 2 || dirs[0] != nodeDir || dirs[1] != npmDir {
		t.Errorf("want [%s %s], got %v", nodeDir, npmDir, dirs)
	}
}

// lerd's own bin dir holds the fnm node shim when Node is managed; a stale
// shim must never count as a system Node, so the PATH walk skips it and finds
// the real install further down.
func TestSystemNodeBinDirs_skipsLerdBinDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("HOME", filepath.Join(tmp, "home"))

	lerdBin := filepath.Join(tmp, "lerd", "bin")
	writeExec(t, lerdBin, "node")
	writeExec(t, lerdBin, "npm")
	realBin := filepath.Join(tmp, "real")
	writeExec(t, realBin, "node")
	writeExec(t, realBin, "npm")
	t.Setenv("PATH", lerdBin+string(os.PathListSeparator)+realBin)

	dirs := SystemNodeBinDirs()
	if len(dirs) != 1 || dirs[0] != realBin {
		t.Errorf("want [%s], got %v", realBin, dirs)
	}
}

// A bare distro node without npm (Ubuntu's node package) must not shadow a
// version-manager install that carries the full toolchain — the reported
// failure is `npm: command not found`, so npm coverage decides precedence.
func TestSystemNodeBinDirs_nodeOnlyPathLosesToNvm(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")

	bareDir := filepath.Join(tmp, "usrbin")
	writeExec(t, bareDir, "node")
	t.Setenv("PATH", bareDir)

	nvmBin := filepath.Join(home, ".nvm", "versions", "node", "v22.9.1", "bin")
	writeExec(t, nvmBin, "node")
	writeExec(t, nvmBin, "npm")

	dirs := SystemNodeBinDirs()
	if len(dirs) != 1 || dirs[0] != nvmBin {
		t.Errorf("want [%s], got %v", nvmBin, dirs)
	}
}

// With npm nowhere at all, the node-only PATH dir is still returned so a
// `node server.js` host-proxy worker keeps running.
func TestSystemNodeBinDirs_nodeOnlyPathIsLastResort(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("NVM_DIR", "")

	bareDir := filepath.Join(tmp, "usrbin")
	writeExec(t, bareDir, "node")
	t.Setenv("PATH", bareDir)

	dirs := SystemNodeBinDirs()
	if len(dirs) != 1 || dirs[0] != bareDir {
		t.Errorf("want [%s], got %v", bareDir, dirs)
	}
}

// A shell-hook nvm is invisible to a daemon's PATH; the resolver must still
// find its newest installed version directory.
func TestSystemNodeBinDirs_nvmFallbackPicksHighestVersion(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")
	t.Setenv("PATH", filepath.Join(tmp, "empty"))

	for _, v := range []string{"v20.10.0", "v22.9.1"} {
		bin := filepath.Join(home, ".nvm", "versions", "node", v, "bin")
		writeExec(t, bin, "node")
		writeExec(t, bin, "npm")
	}

	want := filepath.Join(home, ".nvm", "versions", "node", "v22.9.1", "bin")
	dirs := SystemNodeBinDirs()
	if len(dirs) != 1 || dirs[0] != want {
		t.Errorf("want [%s], got %v", want, dirs)
	}
}

// When nvm's default alias names an installed version, that version wins over
// the newest one, matching what `nvm use default` would run.
func TestSystemNodeBinDirs_nvmDefaultAliasWins(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")
	t.Setenv("PATH", filepath.Join(tmp, "empty"))

	for _, v := range []string{"v20.10.0", "v22.9.1"} {
		bin := filepath.Join(home, ".nvm", "versions", "node", v, "bin")
		writeExec(t, bin, "node")
		writeExec(t, bin, "npm")
	}
	aliasDir := filepath.Join(home, ".nvm", "alias")
	if err := os.MkdirAll(aliasDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(aliasDir, "default"), []byte("v20.10.0\n"), 0644)

	want := filepath.Join(home, ".nvm", "versions", "node", "v20.10.0", "bin")
	dirs := SystemNodeBinDirs()
	if len(dirs) != 1 || dirs[0] != want {
		t.Errorf("want [%s], got %v", want, dirs)
	}
}

func TestSystemNodeBinDirs_fnmUserInstall(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")
	t.Setenv("PATH", filepath.Join(tmp, "empty"))

	bin := filepath.Join(home, ".local", "share", "fnm", "node-versions", "v22.9.1", "installation", "bin")
	writeExec(t, bin, "node")
	writeExec(t, bin, "npm")

	dirs := SystemNodeBinDirs()
	if len(dirs) != 1 || dirs[0] != bin {
		t.Errorf("want [%s], got %v", bin, dirs)
	}
}

// `nvm alias default 20` is the common way to pin a major, and `nvm use
// default` then runs the newest installed 20.x. Resolving it to the newest
// version overall would hand a worker a different major than lerd's own npm.
func TestSystemNodeBinDirs_nvmPartialDefaultAlias(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")
	t.Setenv("PATH", filepath.Join(tmp, "empty"))

	for _, v := range []string{"v20.10.0", "v20.20.2", "v24.18.0"} {
		writeExec(t, filepath.Join(home, ".nvm", "versions", "node", v, "bin"), "node")
		writeExec(t, filepath.Join(home, ".nvm", "versions", "node", v, "bin"), "npm")
	}
	aliasDir := filepath.Join(home, ".nvm", "alias")
	if err := os.MkdirAll(aliasDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aliasDir, "default"), []byte("20\n"), 0644); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(home, ".nvm", "versions", "node", "v20.20.2", "bin")
	if dirs := SystemNodeBinDirs(); len(dirs) != 1 || dirs[0] != want {
		t.Errorf("want [%s], got %v", want, dirs)
	}

	// The same pin written the other common way.
	if err := os.WriteFile(filepath.Join(aliasDir, "default"), []byte("v20\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if dirs := SystemNodeBinDirs(); len(dirs) != 1 || dirs[0] != want {
		t.Errorf("v-prefixed pin: want [%s], got %v", want, dirs)
	}
}

// A non version alias cannot be resolved without running nvm, so the newest
// install stays the fallback rather than leaving the worker with no Node.
func TestSystemNodeBinDirs_nvmSymbolicDefaultAlias(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")
	t.Setenv("PATH", filepath.Join(tmp, "empty"))

	for _, v := range []string{"v20.20.2", "v24.18.0"} {
		writeExec(t, filepath.Join(home, ".nvm", "versions", "node", v, "bin"), "node")
		writeExec(t, filepath.Join(home, ".nvm", "versions", "node", v, "bin"), "npm")
	}
	aliasDir := filepath.Join(home, ".nvm", "alias")
	if err := os.MkdirAll(aliasDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aliasDir, "default"), []byte("lts/iron\n"), 0644); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(home, ".nvm", "versions", "node", "v24.18.0", "bin")
	if dirs := SystemNodeBinDirs(); len(dirs) != 1 || dirs[0] != want {
		t.Errorf("want [%s], got %v", want, dirs)
	}
}

// A site pinned to a major is what `lerd npm` runs in that directory, so an
// unmanaged worker for it has to get the same major out of nvm, not whatever
// the user's default alias happens to be.
func TestSystemNodeBinDirsFor_pinnedVersionBeatsNvmDefault(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")
	t.Setenv("PATH", filepath.Join(tmp, "empty"))

	for _, v := range []string{"v20.20.2", "v22.23.1"} {
		writeExec(t, filepath.Join(home, ".nvm", "versions", "node", v, "bin"), "node")
		writeExec(t, filepath.Join(home, ".nvm", "versions", "node", v, "bin"), "npm")
	}
	aliasDir := filepath.Join(home, ".nvm", "alias")
	if err := os.MkdirAll(aliasDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aliasDir, "default"), []byte("20\n"), 0644); err != nil {
		t.Fatal(err)
	}
	setNodeManager(t, "nvm")

	want := filepath.Join(home, ".nvm", "versions", "node", "v22.23.1", "bin")
	if dirs := SystemNodeBinDirsFor("22"); len(dirs) != 1 || dirs[0] != want {
		t.Errorf("want [%s], got %v", want, dirs)
	}

	// Not installed in nvm: lerd must not install into the user's nvm, so the
	// default is the next best thing rather than nothing.
	fallback := filepath.Join(home, ".nvm", "versions", "node", "v20.20.2", "bin")
	if dirs := SystemNodeBinDirsFor("18"); len(dirs) != 1 || dirs[0] != fallback {
		t.Errorf("want [%s], got %v", fallback, dirs)
	}
}

// Without nvm configured the pin changes nothing: lerd has no way to select a
// version out of an arbitrary system install.
func TestSystemNodeBinDirsFor_ignoredWithoutNvm(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")

	distro := filepath.Join(tmp, "usr", "bin")
	writeExec(t, distro, "node")
	writeExec(t, distro, "npm")
	t.Setenv("PATH", distro)

	if dirs := SystemNodeBinDirsFor("22"); len(dirs) != 1 || dirs[0] != distro {
		t.Errorf("want [%s], got %v", distro, dirs)
	}
}

// setNodeManager persists node.manager into a config the test owns.
func setNodeManager(t *testing.T, manager string) {
	t.Helper()
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.SetNodeManager(manager)
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
}

// With nvm configured, `lerd npm` runs through nvm, so a worker must not pick
// up an unrelated node from PATH and end up on a different major.
func TestSystemNodeBinDirs_configuredNvmBeatsPathNode(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")

	distro := filepath.Join(tmp, "usr", "bin")
	writeExec(t, distro, "node")
	writeExec(t, distro, "npm")
	t.Setenv("PATH", distro)

	nvmBin := filepath.Join(home, ".nvm", "versions", "node", "v22.9.1", "bin")
	writeExec(t, nvmBin, "node")
	writeExec(t, nvmBin, "npm")
	setNodeManager(t, "nvm")

	dirs := SystemNodeBinDirs()
	if len(dirs) != 1 || dirs[0] != nvmBin {
		t.Errorf("want [%s], got %v", nvmBin, dirs)
	}
}

// An nvm with nothing installed in it resolves nothing, so the usual order
// still applies rather than leaving the worker with no Node at all.
func TestSystemNodeBinDirs_configuredNvmWithoutVersionsFallsBack(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")

	distro := filepath.Join(tmp, "usr", "bin")
	writeExec(t, distro, "node")
	writeExec(t, distro, "npm")
	t.Setenv("PATH", distro)

	if err := os.MkdirAll(filepath.Join(home, ".nvm"), 0755); err != nil {
		t.Fatal(err)
	}
	setNodeManager(t, "nvm")

	dirs := SystemNodeBinDirs()
	if len(dirs) != 1 || dirs[0] != distro {
		t.Errorf("want [%s], got %v", distro, dirs)
	}
}

// fnm is lerd's own bundled tool, not the user's install, so an unmanaged host
// keeps resolving the user's own Node first exactly as before.
func TestSystemNodeBinDirs_configuredFnmKeepsPathFirst(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", home)
	t.Setenv("NVM_DIR", "")

	distro := filepath.Join(tmp, "usr", "bin")
	writeExec(t, distro, "node")
	writeExec(t, distro, "npm")
	t.Setenv("PATH", distro)

	nvmBin := filepath.Join(home, ".nvm", "versions", "node", "v22.9.1", "bin")
	writeExec(t, nvmBin, "node")
	writeExec(t, nvmBin, "npm")
	setNodeManager(t, "fnm")

	dirs := SystemNodeBinDirs()
	if len(dirs) != 1 || dirs[0] != distro {
		t.Errorf("want [%s], got %v", distro, dirs)
	}
}

func TestSystemNodeBinDirs_nothingResolvable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("NVM_DIR", "")
	t.Setenv("PATH", filepath.Join(tmp, "empty"))

	if dirs := SystemNodeBinDirs(); dirs != nil {
		t.Errorf("want nil, got %v", dirs)
	}
}

func TestCommandUsesNode(t *testing.T) {
	cases := []struct {
		command string
		want    bool
	}{
		{"npm run dev", true},
		{"npx vite", true},
		{"node server.js", true},
		{"yarn dev", true},
		{"pnpm dev", true},
		{"env PORT=3000 npm run dev", true},
		{"npm run build && npm run preview", true},
		{"composer install && npm run dev", true},
		{"python manage.py runserver", false},
		{"./server --port 8000", false},
		{"echo 'npm run dev'", false},
		{"", false},
	}
	for _, c := range cases {
		if got := CommandUsesNode(c.command); got != c.want {
			t.Errorf("CommandUsesNode(%q) = %v, want %v", c.command, got, c.want)
		}
	}
}
