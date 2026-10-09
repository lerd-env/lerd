package git

import (
	"os"
	"path/filepath"
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
