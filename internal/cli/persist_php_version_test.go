package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestPersistPHPVersion_NestedWorktreePinsTheWorktree covers switching version from
// inside a worktree checked out under its parent site. The parent matches by path
// prefix, so without resolving the worktree the pin lands at the parent root: the
// branch keeps its old version and the parent is repinned as a side effect.
func TestPersistPHPVersion_NestedWorktreePinsTheWorktree(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	site := filepath.Join(t.TempDir(), "app")
	wt := filepath.Join(site, "wt", "feature")
	makeWorktree(t, site, wt, "feature")

	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}

	persistPHPVersion(wt, "8.3")

	if got, err := phpVersionForDir(wt); err != nil || got != "8.3" {
		t.Errorf("phpVersionForDir = %q (err %v), want %q", got, err, "8.3")
	}
	assertPinFile(t, wt, "8.3")
	if _, err := os.Stat(filepath.Join(site, ".php-version")); !os.IsNotExist(err) {
		t.Errorf("parent site was repinned by a switch made in the worktree")
	}
}

// TestPersistPHPVersion_SiblingWorktreePinsTheWorktree covers the same switch from a
// worktree checked out beside its project. It matches no registered site, so the pin
// file already landed here, but only the .lerd.yaml override actually takes effect.
func TestPersistPHPVersion_SiblingWorktreePinsTheWorktree(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	root := t.TempDir()
	site := filepath.Join(root, "app")
	wt := filepath.Join(root, "app-feature")
	makeWorktree(t, site, wt, "feature")

	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}

	persistPHPVersion(wt, "8.3")

	if got, err := phpVersionForDir(wt); err != nil || got != "8.3" {
		t.Errorf("phpVersionForDir = %q (err %v), want %q", got, err, "8.3")
	}
	assertPinFile(t, wt, "8.3")
}

// TestPersistPHPVersion_SubdirOfWorktreePinsTheWorktreeRoot resolves the checkout
// from a directory inside it, since commands run from anywhere in the project.
func TestPersistPHPVersion_SubdirOfWorktreePinsTheWorktreeRoot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	site := filepath.Join(t.TempDir(), "app")
	wt := filepath.Join(site, "wt", "feature")
	makeWorktree(t, site, wt, "feature")
	sub := filepath.Join(wt, "app", "Http")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}

	persistPHPVersion(sub, "8.3")

	assertPinFile(t, wt, "8.3")
	if _, err := os.Stat(filepath.Join(sub, ".php-version")); !os.IsNotExist(err) {
		t.Errorf("pin landed in the subdirectory instead of the worktree root")
	}
}

// TestPersistPHPVersion_PlainSitePinsTheSiteRoot keeps the ordinary case working:
// from a subdirectory of a registered site the pin belongs at the project root.
func TestPersistPHPVersion_PlainSitePinsTheSiteRoot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	site := filepath.Join(t.TempDir(), "app")
	sub := filepath.Join(site, "app", "Http")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}

	persistPHPVersion(sub, "8.3")

	assertPinFile(t, site, "8.3")
}

func assertPinFile(t *testing.T, dir, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".php-version"))
	if err != nil {
		t.Fatalf("reading .php-version in %s: %v", dir, err)
	}
	if got := strings.TrimSpace(string(data)); got != want {
		t.Errorf(".php-version = %q, want %q", got, want)
	}
}
