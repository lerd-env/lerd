package git

import (
	"path/filepath"
	"strings"
	"testing"
)

// pullFixture returns a checkout tracking a bare remote, plus a second clone
// that can push to that remote behind the checkout's back.
func pullFixture(t *testing.T) (dir, other string) {
	t.Helper()
	remote, dir, other := t.TempDir(), t.TempDir(), t.TempDir()
	gitRun(t, remote, "init", "-q", "--bare")
	gitRun(t, dir, "init", "-q")
	write(t, filepath.Join(dir, "a"), "base")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "base")
	gitRun(t, dir, "remote", "add", "origin", remote)
	gitRun(t, dir, "push", "-q", "-u", "origin", "main")
	gitRun(t, other, "clone", "-q", remote, ".")
	return dir, other
}

func head(t *testing.T, dir string) string {
	t.Helper()
	out, err := Output(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out)
}

func TestPull_fastForwards(t *testing.T) {
	dir, other := pullFixture(t)
	write(t, filepath.Join(other, "a"), "theirs")
	gitRun(t, other, "commit", "-q", "-am", "theirs")
	gitRun(t, other, "push", "-q")

	if _, err := Pull(dir); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if head(t, dir) != head(t, other) {
		t.Fatal("checkout did not reach the remote's commit")
	}
}

// A diverged branch needs a merge or rebase, which is the user's call, not ours.
func TestPull_refusesDivergedBranch(t *testing.T) {
	dir, other := pullFixture(t)
	write(t, filepath.Join(other, "a"), "theirs")
	gitRun(t, other, "commit", "-q", "-am", "theirs")
	gitRun(t, other, "push", "-q")
	write(t, filepath.Join(dir, "b"), "ours")
	gitRun(t, dir, "add", "b")
	gitRun(t, dir, "commit", "-q", "-m", "ours")
	before := head(t, dir)

	_, err := Pull(dir)
	if err == nil {
		t.Fatal("want refusal on a diverged branch")
	}
	if head(t, dir) != before {
		t.Error("diverged checkout was moved")
	}
	if st, _ := ReadStatus(dir); st.Conflicted != 0 {
		t.Error("pull left conflicts behind")
	}
	if strings.Contains(err.Error(), "hint:") || !strings.Contains(err.Error(), "fast-forward") {
		t.Errorf("want only git's reason, got %q", err)
	}
}

// The error carries git's own words so the UI can say why.
func TestPull_errorNamesGitsReason(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	write(t, filepath.Join(dir, "a"), "x")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "x")

	_, err := Pull(dir)
	if err == nil || !strings.Contains(err.Error(), "no tracking information") {
		t.Fatalf("want git's no-upstream message, got %v", err)
	}
}

func TestFetch_updatesBehindCount(t *testing.T) {
	dir, other := pullFixture(t)
	gitRun(t, other, "commit", "-q", "--allow-empty", "-m", "theirs")
	gitRun(t, other, "push", "-q")

	if _, err := Fetch(dir); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if st, _ := ReadStatus(dir); st.Behind != 1 {
		t.Fatalf("want behind 1 after fetch, got %+v", st)
	}
}

func TestPush_sendsLocalCommits(t *testing.T) {
	dir, other := pullFixture(t)
	gitRun(t, dir, "commit", "-q", "--allow-empty", "-m", "ours")

	if _, err := Push(dir); err != nil {
		t.Fatalf("Push: %v", err)
	}
	gitRun(t, other, "pull", "-q")
	if head(t, other) != head(t, dir) {
		t.Fatal("remote did not receive the commit")
	}
}

// Push is never forced: when the remote moved on, someone else's work wins.
func TestPush_refusesWhenRemoteMoved(t *testing.T) {
	dir, other := pullFixture(t)
	gitRun(t, other, "commit", "-q", "--allow-empty", "-m", "theirs")
	gitRun(t, other, "push", "-q")
	theirs := head(t, other)
	gitRun(t, dir, "commit", "-q", "--allow-empty", "-m", "ours")

	if _, err := Push(dir); err == nil {
		t.Fatal("want rejection when the remote moved on")
	}
	gitRun(t, other, "pull", "-q")
	if head(t, other) != theirs {
		t.Error("remote branch was overwritten")
	}
}

func branchOf(t *testing.T, dir string) string {
	t.Helper()
	out, err := Output(dir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out)
}

func TestSwitch_localBranch(t *testing.T) {
	dir, _ := pullFixture(t)
	gitRun(t, dir, "branch", "dev")

	if _, err := Switch(dir, "dev"); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if got := branchOf(t, dir); got != "dev" {
		t.Fatalf("on %q, want dev", got)
	}
}

// A branch only on the remote is checked out as a local branch tracking it.
func TestSwitch_remoteOnlyBranch(t *testing.T) {
	dir, other := pullFixture(t)
	gitRun(t, other, "push", "-q", "origin", "main:release")
	gitRun(t, dir, "fetch", "-q")

	if _, err := Switch(dir, "origin/release"); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if got := branchOf(t, dir); got != "release" {
		t.Fatalf("on %q, want release", got)
	}
	if st, _ := ReadStatus(dir); !st.Upstream {
		t.Error("new local branch does not track the remote one")
	}
}

// Uncommitted work the other branch would overwrite stays put; git refuses.
func TestSwitch_refusesToLoseLocalChanges(t *testing.T) {
	dir, _ := pullFixture(t)
	gitRun(t, dir, "checkout", "-q", "-b", "dev")
	write(t, filepath.Join(dir, "a"), "dev")
	gitRun(t, dir, "commit", "-q", "-am", "dev")
	gitRun(t, dir, "checkout", "-q", "main")
	write(t, filepath.Join(dir, "a"), "unsaved")

	if _, err := Switch(dir, "dev"); err == nil {
		t.Fatal("want refusal")
	}
	if got := branchOf(t, dir); got != "main" {
		t.Errorf("moved to %q", got)
	}
}

// A new branch starts at its base but tracks nothing: pushing it must not
// land on the base's upstream.
func TestSwitchNew(t *testing.T) {
	dir, other := pullFixture(t)
	gitRun(t, other, "push", "-q", "origin", "main:release")
	gitRun(t, dir, "fetch", "-q")

	if _, err := SwitchNew(dir, "feature/x", "origin/release"); err != nil {
		t.Fatalf("SwitchNew: %v", err)
	}
	if got := branchOf(t, dir); got != "feature/x" {
		t.Fatalf("on %q, want feature/x", got)
	}
	if st, _ := ReadStatus(dir); st.Upstream {
		t.Error("new branch tracks its base")
	}
	if _, err := SwitchNew(dir, "feature/x", ""); err == nil {
		t.Error("want a refusal for a branch that already exists")
	}
}

func TestSwitchNew_fromCurrentBranch(t *testing.T) {
	dir, _ := pullFixture(t)
	before := head(t, dir)
	if _, err := SwitchNew(dir, "spike", ""); err != nil {
		t.Fatalf("SwitchNew: %v", err)
	}
	if branchOf(t, dir) != "spike" || head(t, dir) != before {
		t.Fatal("want spike at the commit main was on")
	}
}
