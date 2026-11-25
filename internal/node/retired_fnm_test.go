package node

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func nodeHost(t *testing.T, saved string, fnm bool) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	prev := miseDirs
	miseDirs = func() []string { return nil }
	t.Cleanup(func() { miseDirs = prev })
	if saved != "" {
		setNodeManager(t, saved)
	}
	if fnm {
		if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(config.BinDir(), "fnm"), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// mise is the default; an unset manager is fnm only for an install from before
// the setting that still has lerd's own.
func TestActive_DefaultsToMise(t *testing.T) {
	cases := []struct {
		saved string
		fnm   bool
		want  string
	}{
		{"", false, "mise"},
		{"", true, "fnm"},
		{"fnm", true, "fnm"},
		{"fnm", false, "fnm"},
		{"mise", true, "mise"},
		{"nvm", true, "nvm"},
		{"nvm", false, "nvm"},
	}
	for _, tc := range cases {
		nodeHost(t, tc.saved, tc.fnm)
		if got := Active().Name(); got != tc.want {
			t.Errorf("saved %q, fnm installed %v: Active() = %q, want %q", tc.saved, tc.fnm, got, tc.want)
		}
	}
}

func TestCheckSelectable(t *testing.T) {
	nodeHost(t, "", false)
	if slices.Contains(Selectable(), "fnm") {
		t.Error("fnm is offered on a host without it")
	}
	if err := CheckSelectable("fnm"); err == nil || !strings.Contains(err.Error(), "no longer installs it") {
		t.Errorf("CheckSelectable(fnm) without fnm = %v", err)
	}
	for _, name := range []string{"", "yarn"} {
		if err := CheckSelectable(name); err == nil || strings.Contains(err.Error(), "fnm") {
			t.Errorf("CheckSelectable(%q) = %v, want a refusal that leaves fnm out", name, err)
		}
	}
	nodeHost(t, "nvm", true)
	for _, name := range []string{"mise", "fnm", "nvm"} {
		if err := CheckSelectable(name); err != nil {
			t.Errorf("CheckSelectable(%q) with fnm installed: %v", name, err)
		}
	}
}

// A user's own fnm is offered and driven like their nvm, but is never the
// default and never lerd's to put shims in front of.
func TestUsersOwnFnm(t *testing.T) {
	for _, rel := range []string{".local/share/fnm", ".fnm", ".cargo/bin", ".local/bin"} {
		nodeHost(t, "", false)
		writeExec(t, filepath.Join(os.Getenv("HOME"), rel), "fnm")
		want := filepath.Join(os.Getenv("HOME"), rel, "fnm")

		if got := UserFnm(); got != want {
			t.Errorf("UserFnm() = %q, want %q", got, want)
		}
		if got := Active().Name(); got != "mise" {
			t.Errorf("%s: an unset manager resolved to %q, want mise", rel, got)
		}
		if err := CheckSelectable("fnm"); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
		m := ManagerByName("fnm")
		if WritesPathShims(m) {
			t.Errorf("%s: shims would mask the user's fnm", rel)
		}
		if got := m.ExecPrefix("22"); !strings.Contains(got, want) {
			t.Errorf("ExecPrefix = %q, want it to run %s", got, want)
		}
	}

	dir := t.TempDir()
	nodeHost(t, "", true)
	writeExec(t, dir, "fnm")
	t.Setenv("PATH", dir)
	if UserFnm() != "" || FnmBinary() != lerdFnm() {
		t.Errorf("lerd's own fnm should win over %s, got %s", dir, FnmBinary())
	}
}

// Declining lerd-managed Node with fnm chosen runs the user's fnm versions, the
// pinned one when they have it.
func TestSystemNodeBinDirs_borrowsTheUsersFnm(t *testing.T) {
	nodeHost(t, "fnm", false)
	base := filepath.Join(os.Getenv("HOME"), ".local", "share", "fnm")
	writeExec(t, base, "fnm")
	for _, v := range []string{"v20.1.0", "v22.9.1"} {
		bin := filepath.Join(base, "node-versions", v, "installation", "bin")
		writeExec(t, bin, "node")
		writeExec(t, bin, "npm")
	}
	for version, want := range map[string]string{"20": "v20.1.0", "22": "v22.9.1", "": "v22.9.1"} {
		if got := borrowedManagerBinDir(version); !strings.Contains(got, want) {
			t.Errorf("borrowedManagerBinDir(%q) = %q, want %s", version, got, want)
		}
	}
	// An fnm that is lerd's is not the user's to borrow.
	writeExec(t, config.BinDir(), "fnm")
	if got := borrowedManagerBinDir("20"); got != "" {
		t.Errorf("borrowed from lerd's own fnm: %q", got)
	}
}
