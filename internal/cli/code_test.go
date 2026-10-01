package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A name resolves through the registry, from anywhere, the way `lerd open` does.
func TestCodeTargetDir_namedSite(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent, _ := makeWorktreeLayout(t, "rapids", "feature")
	writeSitesYAML(t, []config.Site{{Name: "rapids", Path: parent}})

	dir, err := codeTargetDir([]string{"rapids"})
	if err != nil {
		t.Fatalf("codeTargetDir: %v", err)
	}
	if dir != parent {
		t.Errorf("dir = %q, want %q", dir, parent)
	}
	if _, err := codeTargetDir([]string{"nope"}); err == nil {
		t.Error("an unknown site name must error")
	}
}

// No argument opens the site rooted at the current directory.
func TestCodeTargetDir_cwdSite(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent, _ := makeWorktreeLayout(t, "rapids", "feature")
	writeSitesYAML(t, []config.Site{{Name: "rapids", Path: parent}})
	chdir(t, parent)

	dir, err := codeTargetDir(nil)
	if err != nil {
		t.Fatalf("codeTargetDir: %v", err)
	}
	if dir != parent {
		t.Errorf("dir = %q, want the site root %q", dir, parent)
	}
}

// A worktree is a checkout of its own, so it opens itself rather than the parent
// it inherits its registration from.
func TestCodeTargetDir_worktreeOpensItself(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent, wtPath := makeWorktreeLayout(t, "rapids", "feature")
	writeSitesYAML(t, []config.Site{{Name: "rapids", Path: parent}})
	chdir(t, wtPath)

	dir, err := codeTargetDir(nil)
	if err != nil {
		t.Fatalf("codeTargetDir: %v", err)
	}
	// SamePath, not string equality: the worktree comes back through os.Getwd,
	// which resolves the /var symlink macOS puts in front of a temp dir.
	if !config.SamePath(dir, wtPath) {
		t.Errorf("dir = %q, want the worktree %q, not the parent", dir, wtPath)
	}
}

// The editor has to outlive the lerd process and the terminal that ran it: as a
// plain child it shares the shell's process group and dies with the terminal,
// taking the just-opened project window with it.
func TestEditorCmd_runsInItsOwnSession(t *testing.T) {
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()

	cmd := editorCmd([]string{"phpstorm", "/home/u/site"}, null)
	if !reflect.DeepEqual(cmd.SysProcAttr, detachedSysProcAttr()) {
		t.Error("editor must be started detached so a closed terminal cannot kill it")
	}
	if cmd.Stdout != null || cmd.Stderr != null || cmd.Stdin != null {
		t.Error("editor stdio must go to null, not the shell it was launched from")
	}
}

// An unlinked directory in a non-interactive process gets the same advice every
// other directory-scoped command gives, not an editor window on nothing.
func TestCodeTargetDir_unlinkedErrors(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	chdir(t, filepath.Join(t.TempDir()))

	if _, err := codeTargetDir(nil); err == nil || !strings.Contains(err.Error(), "lerd link") {
		t.Errorf("err = %v, want the lerd link advice", err)
	}
}
