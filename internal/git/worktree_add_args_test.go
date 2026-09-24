package git

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The forms docs/features/git-worktrees.md documents, none of which carry a
// path. Every one of them died on git's usage message before this.
func TestDeriveWorktreeAddArgs_fillsInTheDocumentedNewBranchForm(t *testing.T) {
	site := t.TempDir()
	want := WorktreeCheckoutPath(site, "feat-x")

	got := DeriveWorktreeAddArgs(site, []string{"-b", "feat-x"})

	if !reflect.DeepEqual(got, []string{"-b", "feat-x", want}) {
		t.Errorf("args = %v, want -b feat-x %s", got, want)
	}
}

// git's form is `-b <branch> <path> [<start-point>]`, so the path goes between
// the branch and the start point rather than on the end.
func TestDeriveWorktreeAddArgs_putsThePathBeforeAStartPoint(t *testing.T) {
	site := t.TempDir()
	want := WorktreeCheckoutPath(site, "feat-x")

	got := DeriveWorktreeAddArgs(site, []string{"--track", "-b", "feat-x", "origin/feat"})

	if !reflect.DeepEqual(got, []string{"--track", "-b", "feat-x", want, "origin/feat"}) {
		t.Errorf("args = %v, want the path before origin/feat", got)
	}
}

// An existing branch takes git's `<path> <branch>` order, the same shape the
// dashboard builds.
func TestDeriveWorktreeAddArgs_existingBranchGetsPathThenBranch(t *testing.T) {
	site := t.TempDir()
	want := WorktreeCheckoutPath(site, "feature/auth")

	got := DeriveWorktreeAddArgs(site, []string{"feature/auth"})

	if !reflect.DeepEqual(got, []string{want, "feature/auth"}) {
		t.Errorf("args = %v, want %s feature/auth", got, want)
	}
}

// A branch with a slash must not read as a path, or the fix would skip exactly
// the form it exists to repair.
func TestDeriveWorktreeAddArgs_aSlashInABranchIsNotAPath(t *testing.T) {
	if hasWorktreePathArg([]string{"feature/auth"}) {
		t.Error("feature/auth read as a path")
	}
	if !hasWorktreePathArg([]string{"-b", "x", "../demo-x"}) {
		t.Error("../demo-x should read as a path")
	}
}

// Anyone already typing a path keeps the behaviour they have.
func TestDeriveWorktreeAddArgs_leavesAnExplicitPathAlone(t *testing.T) {
	site := t.TempDir()
	in := []string{"-b", "feat-x", "../demo-feat-x"}

	if got := DeriveWorktreeAddArgs(site, in); !reflect.DeepEqual(got, in) {
		t.Errorf("args = %v, want them untouched", got)
	}
}

// Two positionals with no -b could be a path and a commit-ish, or a branch and
// a commit-ish. Guessing is how a wrapper checks out the wrong thing.
func TestDeriveWorktreeAddArgs_passesThroughWhenAmbiguous(t *testing.T) {
	site := t.TempDir()
	in := []string{"--detach", "feat-x", "abc1234"}

	if got := DeriveWorktreeAddArgs(site, in); !reflect.DeepEqual(got, in) {
		t.Errorf("args = %v, want them untouched", got)
	}
}

// The derived path is a child of the site, which is the layout the dashboard
// produces and the one .git/info/exclude is written for.
func TestDeriveWorktreeAddArgs_derivesAChildOfTheSite(t *testing.T) {
	site := t.TempDir()
	got := DeriveWorktreeAddArgs(site, []string{"-b", "feat-x"})

	path := got[len(got)-1]
	if filepath.Dir(path) != site {
		t.Errorf("path %s is not a child of %s", path, site)
	}
	if !strings.HasSuffix(path, filepath.Base(site)+"-feat-x") {
		t.Errorf("path %s does not follow <base>-<branch>", path)
	}
}

func TestWorktreeCheckoutPath(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "myapp")

	// Worktrees live under the parent: <parent>/<parentBase>-<slug>.
	got := WorktreeCheckoutPath(parent, "feature/auth")
	want := filepath.Join(parent, "myapp-feature-auth")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// When the default path already exists, it bumps a numeric suffix.
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}
	got = WorktreeCheckoutPath(parent, "feature/auth")
	if got != want+"-2" {
		t.Fatalf("got %q want %q", got, want+"-2")
	}
}
