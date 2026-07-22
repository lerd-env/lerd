package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A worktree checked out inside its parent site path-prefix-matches the parent
// in SiteRootFor, so the shell must resolve the worktree checkout itself, or it
// opens the parent tree while running the worktree's own PHP and FPM.
func TestShellWorkDir_NestedWorktreeOpensTheWorktree(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	site := filepath.Join(t.TempDir(), "app")
	wt := filepath.Join(site, "wt", "feature")
	makeWorktree(t, site, wt, "feature")
	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}

	if got := shellWorkDir(wt); got != wt {
		t.Errorf("shellWorkDir = %q, want the worktree root %q", got, wt)
	}
}

// From inside a plain registered site the shell opens the site root, wherever in
// the tree the command was run.
func TestShellWorkDir_PlainSiteOpensTheSiteRoot(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	site := filepath.Join(t.TempDir(), "app")
	sub := filepath.Join(site, "app", "Http")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}

	if got := shellWorkDir(sub); got != site {
		t.Errorf("shellWorkDir = %q, want the site root %q", got, site)
	}
}
