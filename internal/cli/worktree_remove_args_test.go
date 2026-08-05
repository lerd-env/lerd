package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// `lerd worktree add -b feat-x` checks the branch out at <project>-feat-x, so
// the symmetric remove has to accept the branch name. git itself only takes a
// path or the checkout's basename and fails with "is not a working tree".
func TestResolveWorktreeArgsMapsABranchToItsCheckout(t *testing.T) {
	site := t.TempDir()
	wt := filepath.Join(filepath.Dir(site), "app-feat-x")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := filepath.Join(site, ".git", "worktrees", "app-feat-x")
	if err := os.MkdirAll(meta, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta, "HEAD"), []byte("ref: refs/heads/feat-x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta, "gitdir"), []byte(filepath.Join(wt, ".git")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := resolveWorktreeArgs(site, []string{"feat-x"})
	if len(got) != 1 || got[0] != wt {
		t.Errorf("resolveWorktreeArgs(feat-x) = %v, want [%s]", got, wt)
	}

	// Flags and an explicit path are passed through untouched.
	got = resolveWorktreeArgs(site, []string{"--force", wt})
	if len(got) != 2 || got[0] != "--force" || got[1] != wt {
		t.Errorf("an explicit path was rewritten: %v", got)
	}

	// A name that matches no worktree is left for git to report on.
	got = resolveWorktreeArgs(site, []string{"nope"})
	if len(got) != 1 || got[0] != "nope" {
		t.Errorf("an unknown name was rewritten: %v", got)
	}
}
