package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseStatus(t *testing.T) {
	out := `# branch.oid 1a2b3c
# branch.head feature
# branch.upstream origin/feature
# branch.ab +2 -1
1 M. N... 100644 100644 100644 aaa bbb staged.php
1 .M N... 100644 100644 100644 aaa bbb modified.php
1 MM N... 100644 100644 100644 aaa bbb both.php
2 R. N... 100644 100644 100644 aaa bbb R100 new.php	old.php
u UU N... 100644 100644 100644 100644 aaa bbb ccc conflict.php
? untracked.php
? other.txt
! ignored.log
`
	got := ParseStatus(out)
	want := Status{Staged: 3, Modified: 2, Untracked: 2, Conflicted: 1, Ahead: 2, Behind: 1}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestParseStatus_clean(t *testing.T) {
	got := ParseStatus("# branch.oid 1a2b3c\n# branch.head main\n")
	if got != (Status{}) {
		t.Fatalf("clean tree should be zero, got %+v", got)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil && args[0] != "merge" {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ReadStatus must agree with git itself on a real repo holding every state.
func TestReadStatus_realRepo(t *testing.T) {
	remote, dir := t.TempDir(), t.TempDir()
	gitRun(t, remote, "init", "-q", "--bare")
	gitRun(t, dir, "init", "-q")
	for _, f := range []string{"a", "b", "c", "conflict"} {
		write(t, filepath.Join(dir, f), f)
	}
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "base")
	gitRun(t, dir, "remote", "add", "origin", remote)
	gitRun(t, dir, "push", "-q", "-u", "origin", "main")

	// One commit on the remote only (behind 1), one local only (ahead 1).
	other := t.TempDir()
	gitRun(t, other, "clone", "-q", remote, ".")
	write(t, filepath.Join(other, "conflict"), "theirs")
	gitRun(t, other, "commit", "-q", "-am", "theirs")
	gitRun(t, other, "push", "-q")
	write(t, filepath.Join(dir, "conflict"), "ours")
	gitRun(t, dir, "commit", "-q", "-am", "ours")
	gitRun(t, dir, "fetch", "-q")

	write(t, filepath.Join(dir, "a"), "staged")
	gitRun(t, dir, "add", "a")
	write(t, filepath.Join(dir, "b"), "modified")
	write(t, filepath.Join(dir, "new1"), "x")
	write(t, filepath.Join(dir, "new2"), "x")

	got, err := ReadStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := Status{Staged: 1, Modified: 1, Untracked: 2, Ahead: 1, Behind: 1}
	if got != want {
		t.Fatalf("before merge: got %+v want %+v", got, want)
	}

	gitRun(t, dir, "stash", "-q", "--include-untracked")
	gitRun(t, dir, "merge", "-q", "origin/main")
	got, err = ReadStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Conflicted != 1 {
		t.Fatalf("after conflicting merge: got %+v, want 1 conflicted", got)
	}
}
