package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBranchDiff(t *testing.T) {
	dir, _ := pullFixture(t)
	write(t, filepath.Join(dir, "gone"), "x")
	gitRun(t, dir, "add", "gone")
	gitRun(t, dir, "commit", "-q", "-m", "gone")
	gitRun(t, dir, "checkout", "-q", "-b", "other")
	gitRun(t, dir, "rm", "-q", "gone")
	write(t, filepath.Join(dir, "a"), "changed")
	write(t, filepath.Join(dir, "new"), "x")
	gitRun(t, dir, "add", "-A", ".")
	gitRun(t, dir, "commit", "-q", "-m", "other")
	gitRun(t, dir, "checkout", "-q", "main")

	got, err := DiffNameStatus(dir, "HEAD", "other")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]byte{"a": 'M', "new": 'A', "gone": 'D'}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for f, s := range want {
		if got[f] != s {
			t.Errorf("%s: got %c want %c", f, got[f], s)
		}
	}
	if !FileAtRef(dir, "other", "new") || FileAtRef(dir, "HEAD", "new") {
		t.Error("FileAtRef disagrees with the trees")
	}
	if a, b := AheadBehind(dir, "HEAD", "other"); a != 0 || b != 1 {
		t.Errorf("ahead/behind = %d/%d, want 0/1", a, b)
	}
}

func TestBranchDates(t *testing.T) {
	dir, _ := pullFixture(t)
	gitRun(t, dir, "branch", "dev")
	gitRun(t, dir, "fetch", "-q")

	got := BranchDates(dir)
	for _, b := range []string{"main", "dev", "origin/main"} {
		if got[b] == 0 {
			t.Errorf("%s has no date: %v", b, got)
		}
	}
	if _, ok := got["origin/HEAD"]; ok {
		t.Error("symbolic origin/HEAD should not be listed")
	}
}

func TestDirtyFiles(t *testing.T) {
	dir, _ := pullFixture(t)
	write(t, filepath.Join(dir, "a"), "edited")
	if err := os.MkdirAll(filepath.Join(dir, "sub dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "sub dir", "new file"), "x")

	got := DirtyFiles(dir)
	want := map[string]bool{"a": true, "sub dir/new file": true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, f := range got {
		if !want[f] {
			t.Errorf("unexpected %q in %v", f, got)
		}
	}
}

// Names git would quote (non-ASCII, spaces) come back raw, matching DirtyFiles.
func TestDiffNameStatus_rawNames(t *testing.T) {
	dir, _ := pullFixture(t)
	gitRun(t, dir, "checkout", "-q", "-b", "other")
	write(t, filepath.Join(dir, "migrație nouă.php"), "x")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "other")
	gitRun(t, dir, "checkout", "-q", "main")

	got, err := DiffNameStatus(dir, "HEAD", "other")
	if err != nil {
		t.Fatal(err)
	}
	if got["migrație nouă.php"] != 'A' {
		t.Fatalf("got %q, want the raw name added", got)
	}
}

func TestRepoPrefix(t *testing.T) {
	dir, _ := pullFixture(t)
	if err := os.MkdirAll(filepath.Join(dir, "apps", "shop"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := RepoPrefix(filepath.Join(dir, "apps", "shop")); got != "apps/shop/" {
		t.Errorf("subfolder prefix = %q", got)
	}
	if got := RepoPrefix(dir); got != "" {
		t.Errorf("root prefix = %q", got)
	}
}

// The label names what will run once the target is checked out, so it reads
// the target's lockfiles, not the current checkout's.
func TestJSInstallCommandAt(t *testing.T) {
	dir, _ := pullFixture(t)
	write(t, filepath.Join(dir, "package-lock.json"), "{}")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "npm")
	gitRun(t, dir, "checkout", "-q", "-b", "pnpm")
	gitRun(t, dir, "rm", "-q", "package-lock.json")
	write(t, filepath.Join(dir, "pnpm-lock.yaml"), "x")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "pnpm")
	gitRun(t, dir, "checkout", "-q", "main")

	if got := JSInstallCommandAt(dir, "pnpm", ""); got != "pnpm install" {
		t.Errorf("target branch label = %q, want pnpm install", got)
	}
	if got := JSInstallCommandAt(dir, "HEAD", ""); got != "npm ci" {
		t.Errorf("current branch label = %q, want npm ci", got)
	}
}

// Every branch is a valid base, including ones open in a worktree.
func TestBranches(t *testing.T) {
	dir, _ := pullFixture(t)
	gitRun(t, dir, "branch", "dev")
	gitRun(t, dir, "fetch", "-q")
	gitRun(t, dir, "worktree", "add", "-q", filepath.Join(t.TempDir(), "wt"), "dev")

	local, remote := Branches(dir)
	if strings.Join(local, ",") != "dev,main" {
		t.Errorf("local = %v", local)
	}
	if strings.Join(remote, ",") != "origin/main" {
		t.Errorf("remote = %v", remote)
	}
}

// Only the remotes' symbolic HEADs are dropped; a branch may itself end in HEAD.
func TestBranches_keepsABranchNamedHEAD(t *testing.T) {
	dir, _ := pullFixture(t)
	gitRun(t, dir, "branch", "feature/HEAD")
	gitRun(t, dir, "remote", "set-head", "origin", "main")

	local, remote := Branches(dir)
	if !strings.Contains(strings.Join(local, ","), "feature/HEAD") {
		t.Errorf("local = %v, want feature/HEAD kept", local)
	}
	if strings.Contains(strings.Join(remote, ","), "origin/HEAD") {
		t.Errorf("remote = %v, symbolic origin/HEAD should go", remote)
	}
	if _, ok := BranchDates(dir)["feature/HEAD"]; !ok {
		t.Error("BranchDates dropped feature/HEAD")
	}
}
