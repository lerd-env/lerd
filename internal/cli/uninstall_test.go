package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── removeMarkedBlock ────────────────────────────────────────────────────────

const testMarker = "# Added by Lerd installer"

func TestRemoveMarkedBlock_removesMarkerAndNextLine(t *testing.T) {
	tmp := t.TempDir()
	rc := filepath.Join(tmp, ".bashrc")

	content := "existing line\n" +
		testMarker + "\n" +
		`export PATH="/home/user/.local/share/lerd/bin:$PATH"` + "\n" +
		"another line\n"

	os.WriteFile(rc, []byte(content), 0644)
	removeMarkedBlock(rc, testMarker, 1)

	got, _ := os.ReadFile(rc)
	if strings.Contains(string(got), testMarker) {
		t.Error("marker line should have been removed")
	}
	if strings.Contains(string(got), "lerd/bin") {
		t.Error("PATH export line should have been removed")
	}
	if !strings.Contains(string(got), "existing line") {
		t.Error("unrelated lines should be preserved")
	}
	if !strings.Contains(string(got), "another line") {
		t.Error("lines after the block should be preserved")
	}
}

func TestRemoveMarkedBlock_noMarker_noChange(t *testing.T) {
	tmp := t.TempDir()
	rc := filepath.Join(tmp, ".bashrc")

	content := "line one\nline two\n"
	os.WriteFile(rc, []byte(content), 0644)
	removeMarkedBlock(rc, testMarker, 1)

	got, _ := os.ReadFile(rc)
	if string(got) != content {
		t.Errorf("file should be unchanged, got:\n%s", got)
	}
}

func TestRemoveMarkedBlock_missingFile_noError(t *testing.T) {
	// Must not panic or return an error — the function is best-effort.
	removeMarkedBlock("/tmp/lerd-test-nonexistent-file-xyz", testMarker, 1)
}

func TestRemoveMarkedBlock_markerAtEndOfFile(t *testing.T) {
	tmp := t.TempDir()
	rc := filepath.Join(tmp, ".zshrc")

	content := "source ~/.profile\n" + testMarker + "\n"
	os.WriteFile(rc, []byte(content), 0644)
	removeMarkedBlock(rc, testMarker, 1)

	got, _ := os.ReadFile(rc)
	if strings.Contains(string(got), testMarker) {
		t.Error("marker should have been removed")
	}
	if !strings.Contains(string(got), "source ~/.profile") {
		t.Error("preceding lines should be preserved")
	}
}

func TestRemoveMarkedBlock_onlyMarker(t *testing.T) {
	tmp := t.TempDir()
	rc := filepath.Join(tmp, ".bashrc")

	os.WriteFile(rc, []byte(testMarker+"\n"), 0644)
	removeMarkedBlock(rc, testMarker, 1)

	got, _ := os.ReadFile(rc)
	if strings.Contains(string(got), testMarker) {
		t.Error("marker should have been removed from single-line file")
	}
}

// ── removeShellEntry ─────────────────────────────────────────────────────────

func TestRemoveShellEntry_bashrc(t *testing.T) {
	tmp := t.TempDir()

	// Simulate a home directory with a .bashrc containing the Lerd PATH block.
	bashrc := filepath.Join(tmp, ".bashrc")
	os.WriteFile(bashrc, []byte(
		"# existing config\n"+
			"# Added by Lerd installer\n"+
			`export PATH="/home/user/.local/share/lerd/bin:$PATH"`+"\n",
	), 0644)

	// Point HOME at the temp dir so removeShellEntry reads our fake rc files.
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	removeShellEntry()

	got, _ := os.ReadFile(bashrc)
	if strings.Contains(string(got), "Added by Lerd installer") {
		t.Error("Lerd marker should have been removed from .bashrc")
	}
	if strings.Contains(string(got), "lerd/bin") {
		t.Error("Lerd PATH export should have been removed from .bashrc")
	}
	if !strings.Contains(string(got), "# existing config") {
		t.Error("pre-existing config should be preserved")
	}
}

func TestRemoveShellEntry_fishConfig(t *testing.T) {
	tmp := t.TempDir()
	fishDir := filepath.Join(tmp, ".config", "fish", "conf.d")
	os.MkdirAll(fishDir, 0755)

	fishConf := filepath.Join(fishDir, "lerd.fish")
	os.WriteFile(fishConf, []byte(
		"# Added by Lerd installer\n"+
			"fish_add_path /home/user/.local/share/lerd/bin\n",
	), 0644)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	removeShellEntry()

	got, _ := os.ReadFile(fishConf)
	if strings.Contains(string(got), "Added by Lerd installer") {
		t.Error("Lerd marker should have been removed from fish config")
	}
}

// TestRemoveShellEntry_removesAllLerdInstallerMarkers pins the fix for a
// user-reported regression: `lerd install` (the Go binary) writes two
// extra marker blocks beyond `install.sh`'s "# Added by Lerd installer"
// — "# Lerd" (PATH export) and "# Lerd completions" (fpath + autoload).
// Uninstall used to match only the first marker, leaving the other two
// behind on every install path that went through `lerd install` (which
// is the common case).
func TestRemoveShellEntry_removesAllLerdInstallerMarkers(t *testing.T) {
	tmp := t.TempDir()
	zshrc := filepath.Join(tmp, ".zshrc")
	os.WriteFile(zshrc, []byte(
		"alias gco='git checkout'\n"+
			"\n"+
			"# Added by Lerd installer\n"+
			`export PATH="/h/u/.local/bin:$PATH"`+"\n"+
			"\n"+
			"# Lerd\n"+
			`export PATH="/h/u/.local/share/lerd/bin:$PATH"`+"\n"+
			"\n"+
			"# Lerd completions\n"+
			"fpath=(/h/u/.local/share/zsh/site-functions $fpath)\n"+
			"autoload -Uz compinit && compinit\n"+
			"\n"+
			"alias home='cd /h/u'\n",
	), 0644)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	removeShellEntry()

	got, _ := os.ReadFile(zshrc)
	for _, leftover := range []string{
		"Added by Lerd installer",
		"# Lerd\n",
		"# Lerd completions",
		"share/lerd/bin",
		"share/zsh/site-functions",
		"compinit",
	} {
		if strings.Contains(string(got), leftover) {
			t.Errorf("expected %q to be removed; remaining content:\n%s", leftover, got)
		}
	}
	for _, preserved := range []string{"git checkout", "cd /h/u"} {
		if !strings.Contains(string(got), preserved) {
			t.Errorf("expected %q to be preserved", preserved)
		}
	}
}

// TestRemoveShellEntry_fishFileDeletedWhenEmpty pins that an entirely
// lerd-owned fish config file is removed (not left as an empty
// conf.d entry that fish keeps sourcing on every shell start).
func TestRemoveShellEntry_fishFileDeletedWhenEmpty(t *testing.T) {
	tmp := t.TempDir()
	fishDir := filepath.Join(tmp, ".config", "fish", "conf.d")
	os.MkdirAll(fishDir, 0755)
	fishConf := filepath.Join(fishDir, "lerd.fish")
	os.WriteFile(fishConf, []byte(
		"\n# Added by Lerd installer\nfish_add_path /h/u/.local/bin\n",
	), 0644)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	removeShellEntry()

	if _, err := os.Stat(fishConf); !os.IsNotExist(err) {
		body, _ := os.ReadFile(fishConf)
		t.Errorf("expected lerd.fish to be removed; still exists with content:\n%s", body)
	}
}

// TestRemoveShellEntry_fishFileKeptWhenNonEmpty pins the inverse: if
// the user added their own content to lerd.fish beyond our markers, we
// strip our blocks but leave the file alone.
func TestRemoveShellEntry_fishFileKeptWhenNonEmpty(t *testing.T) {
	tmp := t.TempDir()
	fishDir := filepath.Join(tmp, ".config", "fish", "conf.d")
	os.MkdirAll(fishDir, 0755)
	fishConf := filepath.Join(fishDir, "lerd.fish")
	os.WriteFile(fishConf, []byte(
		"# Added by Lerd installer\nfish_add_path /h/u/.local/bin\n\n"+
			"# user-added: alias for personal use\nalias myls 'ls -la'\n",
	), 0644)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	removeShellEntry()

	body, err := os.ReadFile(fishConf)
	if err != nil {
		t.Fatalf("file should have been kept: %v", err)
	}
	if strings.Contains(string(body), "Added by Lerd installer") {
		t.Errorf("expected lerd marker removed, got:\n%s", body)
	}
	if !strings.Contains(string(body), "myls") {
		t.Errorf("user content lost; got:\n%s", body)
	}
}

func TestRemoveShellEntry_noRcFiles_noError(t *testing.T) {
	// Point HOME at an empty dir — no rc files exist, should not panic.
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	removeShellEntry() // must not panic
}

// ── removeInstalledBinaries ──────────────────────────────────────────────────

// mkbin creates an executable stand-in at path.
func mkbin(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestRemoveInstalledBinaries_removesLerdAndTray(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "lerd")
	tray := filepath.Join(dir, "lerd-tray")
	mkbin(t, self)
	mkbin(t, tray)

	removeInstalledBinaries(self)

	if _, err := os.Stat(self); !os.IsNotExist(err) {
		t.Errorf("lerd binary still present")
	}
	if _, err := os.Stat(tray); !os.IsNotExist(err) {
		t.Errorf("lerd-tray still present: uninstall must not leave a tray a desktop entry can launch")
	}
}

func TestRemoveInstalledBinaries_missingTrayIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "lerd")
	mkbin(t, self)

	removeInstalledBinaries(self)

	if _, err := os.Stat(self); !os.IsNotExist(err) {
		t.Errorf("lerd binary still present")
	}
}

func TestRemoveInstalledBinaries_leavesUnrelatedNeighbours(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "lerd")
	other := filepath.Join(dir, "lerdfoo")
	mkbin(t, self)
	mkbin(t, other)

	removeInstalledBinaries(self)

	if _, err := os.Stat(other); err != nil {
		t.Errorf("removed an unrelated neighbour %s: %v", other, err)
	}
}

// ── removeDataDir ────────────────────────────────────────────────────────────

// undeletableDir builds a tree os.RemoveAll cannot clear: the inner directory
// has no write bit, which is what a subuid-owned service tree looks like to us.
func undeletableDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	sub := filepath.Join(dir, "redis")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "dump.rdb"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0755) })
	return dir
}

// fakePodman puts a podman on PATH standing in for the user namespace. It is
// handed the one directory the test built and refuses anything else, so a
// mistake here can never widen into a path the test does not own.
func fakePodman(t *testing.T, owned, script string) {
	t.Helper()
	dir := t.TempDir()
	guard := "#!/bin/sh\ncase \"$4\" in\n  " + owned + ") ;;\n  *) echo \"refusing $4\" >&2; exit 2 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "podman"), []byte(guard+script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRemoveDataDir_removesAPlainTree(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(filepath.Join(dir, "mysql"), 0755); err != nil {
		t.Fatal(err)
	}

	if kept := removeDataDir(dir); kept != "" {
		t.Errorf("removeDataDir = %q, want an empty string", kept)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("%s still present", dir)
	}
}

func TestRemoveDataDir_fallsBackToPodmanUnshare(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes the tree without the fallback")
	}
	dir := undeletableDir(t)
	fakePodman(t, dir, "chmod -R u+w \"$4\" && rm -rf \"$4\"\n")

	if kept := removeDataDir(dir); kept != "" {
		t.Errorf("removeDataDir = %q, want an empty string", kept)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("%s survived the podman unshare fallback", dir)
	}
}

func TestRemoveDataDir_reportsTheDirectoryWhenTheFallbackFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes the tree without the fallback")
	}
	dir := undeletableDir(t)
	fakePodman(t, dir, "exit 1\n")

	if kept := removeDataDir(dir); kept != dir {
		t.Errorf("removeDataDir = %q, want %q so the uninstall can say so", kept, dir)
	}
}

// ── removeScriptInstalledBinaries ─────────────────────────────────────────────

// The package-managed guard exists so lerd never deletes a file out of a Cellar
// or an rpm's file list. It said nothing about ~/.local/bin, which no package
// manager owns, so a machine carrying both installs kept a script-installed
// lerd on PATH pointing at a data directory the same run had just deleted.
func TestRemoveScriptInstalledBinaries_clearsTheInstallDirPair(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	lerd := filepath.Join(binDir, "lerd")
	tray := filepath.Join(binDir, "lerd-tray")
	mkbin(t, lerd)
	mkbin(t, tray)

	removeScriptInstalledBinaries("/usr/bin/lerd")

	for _, p := range []string{lerd, tray} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s survived, so a broken lerd stays on PATH", p)
		}
	}
}

// Nothing here may reach the binary the package manager is responsible for,
// which is the whole point of the guard it runs under.
func TestRemoveScriptInstalledBinaries_neverTouchesThePackagedBinary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(binDir, "lerd")
	mkbin(t, self)

	removeScriptInstalledBinaries(self)

	if _, err := os.Stat(self); err != nil {
		t.Error("the running binary is the package manager's to remove, not ours")
	}
}

// ── removeServiceUnits ───────────────────────────────────────────────────────

// unitListMgr answers the unit listings the removal walks and records nothing
// else; the fake it embeds covers the rest of the interface.
type unitListMgr struct {
	*fakeServiceMgr
	containers []string
	services   []string
	timers     []string
}

func (m *unitListMgr) ListContainerUnits(string) []string { return m.containers }
func (m *unitListMgr) ListServiceUnits(string) []string   { return m.services }
func (m *unitListMgr) ListTimerUnits(string) []string     { return m.timers }

// A unit that is already failed when its file goes stays behind as failed and
// not-found, so an uninstalled lerd went on being listed by systemctl on a
// machine that no longer had one. The installer script has reset them all
// along; this path deleted the files, reloaded and stopped there. It does not
// cover a unit that fails after the uninstall has exited, which is a separate
// problem in how long the stop waits.
func TestRemoveServiceUnits_resetsEveryUnitItRemoved(t *testing.T) {
	swapMgr(t, &unitListMgr{
		fakeServiceMgr: &fakeServiceMgr{},
		containers:     []string{"lerd-mysql", "lerd-nginx"},
		services:       []string{"lerd-mysql", "lerd-ui"},
		timers:         []string{"lerd-backup.timer"},
	})

	var reset []string
	orig := resetFailedUnit
	resetFailedUnit = func(name string) { reset = append(reset, name) }
	t.Cleanup(func() { resetFailedUnit = orig })

	removeServiceUnits()

	want := []string{"lerd-mysql", "lerd-nginx", "lerd-ui", "lerd-backup"}
	if !equalStrings(reset, want) {
		t.Errorf("reset %v, want %v", reset, want)
	}
}

// A keep-data uninstall removes every unit, so a built-in service no site lists
// comes back only because uninstall recorded it. One removed since stays removed.
func TestDefaultPresetsToRefresh_restoresKeptServices(t *testing.T) {
	installed := func(unit string) bool { return unit == "lerd-mysql" }
	removed := func(n string) bool { return n == "mailpit" }
	kept := map[string]bool{"redis": true, "mailpit": true}

	got := defaultPresetsToRefresh([]string{"mailpit", "mysql", "postgres", "redis"}, installed, kept, removed)
	want := []string{"mysql", "redis"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("defaultPresetsToRefresh = %v, want %v", got, want)
	}
}

func TestInstalledDefaultServices_listsOnlyThoseWithAUnit(t *testing.T) {
	installed := func(unit string) bool { return unit == "lerd-redis" || unit == "lerd-mysql" }
	got := installedDefaultServices([]string{"mailpit", "mysql", "redis"}, installed)
	if strings.Join(got, ",") != "mysql,redis" {
		t.Fatalf("installedDefaultServices = %v", got)
	}
}
