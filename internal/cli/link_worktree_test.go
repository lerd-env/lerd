package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// makeWorktreeLayout creates the minimal .git/worktrees/<wt>/ structure that
// gitpkg.DetectWorktrees walks. Returns parent path and worktree path.
func makeWorktreeLayout(t *testing.T, parentName, wtBase string) (string, string) {
	t.Helper()
	root := t.TempDir()
	parent := filepath.Join(root, parentName)
	if err := os.MkdirAll(filepath.Join(parent, ".git", "worktrees", wtBase), 0755); err != nil {
		t.Fatal(err)
	}
	wtPath := filepath.Join(parent, wtBase)
	if err := os.MkdirAll(wtPath, 0755); err != nil {
		t.Fatal(err)
	}
	meta := filepath.Join(parent, ".git", "worktrees", wtBase)
	if err := os.WriteFile(filepath.Join(meta, "HEAD"), []byte("ref: refs/heads/"+wtBase+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta, "gitdir"), []byte(filepath.Join(wtPath, ".git")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return parent, wtPath
}

// writeSitesYAML writes a minimal sites.yaml into the current XDG_DATA_HOME so
// config.LoadSites returns the supplied sites.
func writeSitesYAML(t *testing.T, sites []config.Site) {
	t.Helper()
	dir := filepath.Join(os.Getenv("XDG_DATA_HOME"), "lerd")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := "sites:\n"
	for _, s := range sites {
		body += "  - name: " + s.Name + "\n"
		body += "    path: " + s.Path + "\n"
		body += "    domains:\n      - " + s.Name + ".test\n"
		body += "    php_version: \"8.4\"\n    node_version: \"22\"\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "sites.yaml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestFindOwningWorktree_match(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent, wtPath := makeWorktreeLayout(t, "rapids", "main")
	writeSitesYAML(t, []config.Site{{Name: "rapids", Path: parent}})

	owner, branch, ok := findOwningWorktree(wtPath)
	if !ok {
		t.Fatal("expected worktree to be matched to parent")
	}
	if owner.Name != "rapids" || branch != "main" {
		t.Errorf("got %s/%s, want rapids/main", owner.Name, branch)
	}
}

func TestFindOwningWorktree_noMatchForUnrelatedDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent, _ := makeWorktreeLayout(t, "rapids", "main")
	writeSitesYAML(t, []config.Site{{Name: "rapids", Path: parent}})

	if _, _, ok := findOwningWorktree(t.TempDir()); ok {
		t.Error("unrelated dir must not match a worktree")
	}
}

func TestFindOwningWorktree_skipsParentItself(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent, _ := makeWorktreeLayout(t, "rapids", "main")
	writeSitesYAML(t, []config.Site{{Name: "rapids", Path: parent}})

	if _, _, ok := findOwningWorktree(parent); ok {
		t.Error("the parent path itself must not register as one of its own worktrees")
	}
}

// A path can be spelled two ways when a parent directory is a symlink: macOS
// resolves /var to /private/var and ostree hosts resolve /home to /var/home,
// so the registry and git's own metadata can hold one spelling while os.Getwd
// hands back the other. The lookup has to compare canonical paths or a
// worktree is never matched to its parent on those hosts.
func TestFindOwningWorktree_matchesThroughASymlinkedPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	// Register the site and write git's metadata using the symlinked spelling.
	parent := filepath.Join(link, "rapids")
	wtPath := filepath.Join(parent, "feature")
	meta := filepath.Join(parent, ".git", "worktrees", "feature")
	if err := os.MkdirAll(meta, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wtPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta, "HEAD"), []byte("ref: refs/heads/feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta, "gitdir"), []byte(filepath.Join(wtPath, ".git")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeSitesYAML(t, []config.Site{{Name: "rapids", Path: parent}})

	// The resolved spelling is what os.Getwd reports from inside the checkout.
	resolved, err := filepath.EvalSymlinks(wtPath)
	if err != nil {
		t.Fatal(err)
	}
	owner, branch, ok := findOwningWorktree(resolved)
	if !ok {
		t.Fatal("worktree not matched to its parent through the symlinked path")
	}
	if owner.Name != "rapids" || branch != "feature" {
		t.Errorf("got %s/%s, want rapids/feature", owner.Name, branch)
	}
}
