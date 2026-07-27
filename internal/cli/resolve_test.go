package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestErrNotLinkedMentionsLink(t *testing.T) {
	msg := errNotLinked().Error()
	if !strings.Contains(msg, "run 'lerd link' first") {
		t.Errorf("errNotLinked message changed, callers rely on it: %q", msg)
	}
}

// In a test process stdin is not a terminal, so ensureSiteForCwd must take the
// non-interactive branch and return the consistent error without prompting.
// The package source dir it runs in is not a registered site.
func TestEnsureSiteForCwdNonInteractiveErrors(t *testing.T) {
	_, err := ensureSiteForCwd()
	if err == nil {
		t.Fatal("expected an error for an unlinked directory in non-interactive mode")
	}
	if !strings.Contains(err.Error(), "lerd link") {
		t.Errorf("error should point the user at lerd link, got: %v", err)
	}
}

// chdir moves into dir for the test and restores the old cwd afterwards.
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) }) //nolint:errcheck
}

// A worktree is never registered as a site of its own, so an exact path lookup
// always misses and every directory-scoped command used to dead-end on "run
// lerd link first", advice that can never work because link refuses a worktree.
func TestEnsureSiteAndBranchForCwd_resolvesWorktreeToParent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent, wtPath := makeWorktreeLayout(t, "rapids", "feature")
	writeSitesYAML(t, []config.Site{{Name: "rapids", Path: parent}})
	chdir(t, wtPath)

	site, branch, err := ensureSiteAndBranchForCwd()
	if err != nil {
		t.Fatalf("ensureSiteAndBranchForCwd: %v", err)
	}
	if site.Name != "rapids" {
		t.Errorf("site = %q, want rapids", site.Name)
	}
	if branch != "feature" {
		t.Errorf("branch = %q, want feature", branch)
	}
}

// The registered site itself keeps resolving with no branch, so nothing that
// already worked starts behaving like a worktree.
func TestEnsureSiteAndBranchForCwd_registeredSiteHasNoBranch(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent, _ := makeWorktreeLayout(t, "rapids", "feature")
	writeSitesYAML(t, []config.Site{{Name: "rapids", Path: parent}})
	chdir(t, parent)

	site, branch, err := ensureSiteAndBranchForCwd()
	if err != nil {
		t.Fatalf("ensureSiteAndBranchForCwd: %v", err)
	}
	if site.Name != "rapids" {
		t.Errorf("site = %q, want rapids", site.Name)
	}
	if branch != "" {
		t.Errorf("branch = %q, want empty for the parent checkout", branch)
	}
}

// The worktree fallback must be reached before the link prompt, so a worktree
// resolves in a non-interactive process instead of erroring.
func TestEnsureSiteForCwd_worktreeResolvesWithoutPrompting(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent, wtPath := makeWorktreeLayout(t, "rapids", "feature")
	writeSitesYAML(t, []config.Site{{Name: "rapids", Path: parent}})
	chdir(t, wtPath)

	site, err := ensureSiteForCwd()
	if err != nil {
		t.Fatalf("ensureSiteForCwd: %v", err)
	}
	if site.Name != "rapids" {
		t.Errorf("site = %q, want rapids", site.Name)
	}
}
