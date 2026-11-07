package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func commitFixture(t *testing.T) string {
	t.Helper()
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		t.Setenv(k, v)
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	write(t, filepath.Join(dir, "a"), "base")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "base")
	return dir
}

func TestStage_thenUnstage(t *testing.T) {
	dir := commitFixture(t)
	write(t, filepath.Join(dir, "a"), "changed")
	write(t, filepath.Join(dir, "new"), "new")

	for _, f := range []string{"a", "new"} {
		if err := Stage(dir, f); err != nil {
			t.Fatalf("Stage %s: %v", f, err)
		}
	}
	if got := strings.Fields(mustOut(t, dir, "diff", "--cached", "--name-only")); strings.Join(got, ",") != "a,new" {
		t.Fatalf("staged %v", got)
	}
	if err := Unstage(dir, "a"); err != nil {
		t.Fatalf("Unstage: %v", err)
	}
	if got := strings.TrimSpace(mustOut(t, dir, "diff", "--cached", "--name-only")); got != "new" {
		t.Fatalf("after unstaging a, staged %q", got)
	}
	if readFile(t, filepath.Join(dir, "a")) != "changed" {
		t.Error("unstaging touched the working file")
	}
}

// A deleted file is staged as a deletion, not refused for being missing.
func TestStage_deletion(t *testing.T) {
	dir := commitFixture(t)
	if err := os.Remove(filepath.Join(dir, "a")); err != nil {
		t.Fatal(err)
	}
	if err := Stage(dir, "a"); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if got := strings.TrimSpace(mustOut(t, dir, "diff", "--cached", "--name-status")); got != "D\ta" {
		t.Fatalf("staged %q", got)
	}
}

// A name that looks like a pattern means that one file only.
func TestStage_takesTheNameLiterally(t *testing.T) {
	dir := commitFixture(t)
	write(t, filepath.Join(dir, "*.txt"), "star")
	write(t, filepath.Join(dir, "notes.txt"), "other")
	if err := Stage(dir, "*.txt"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(mustOut(t, dir, "diff", "--cached", "--name-only")); got != "*.txt" {
		t.Fatalf("staged %q", got)
	}
	gitRun(t, dir, "add", "notes.txt")
	if err := Unstage(dir, "*.txt"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(mustOut(t, dir, "diff", "--cached", "--name-only")); got != "notes.txt" {
		t.Errorf("after unstaging *.txt, staged %q", got)
	}
}

// A section's group action moves every file it lists in one call.
func TestStage_manyThenUnstageMany(t *testing.T) {
	dir := commitFixture(t)
	write(t, filepath.Join(dir, "a"), "changed")
	write(t, filepath.Join(dir, "new"), "new")
	write(t, filepath.Join(dir, "*.txt"), "star")
	write(t, filepath.Join(dir, "left"), "left")

	if err := Stage(dir, "a", "new", "*.txt"); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if got := strings.Fields(mustOut(t, dir, "diff", "--cached", "--name-only")); strings.Join(got, ",") != "*.txt,a,new" {
		t.Fatalf("staged %v", got)
	}
	if err := Unstage(dir, "a", "*.txt"); err != nil {
		t.Fatalf("Unstage: %v", err)
	}
	if got := strings.TrimSpace(mustOut(t, dir, "diff", "--cached", "--name-only")); got != "new" {
		t.Fatalf("after unstaging, staged %q", got)
	}
}

func TestStage_needsAFile(t *testing.T) {
	dir := commitFixture(t)
	if err := Stage(dir); err == nil {
		t.Error("Stage with no file: want an error")
	}
	if err := Unstage(dir); err == nil {
		t.Error("Unstage with no file: want an error")
	}
}

func TestUnstage_beforeTheFirstCommit(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	write(t, filepath.Join(dir, "a"), "new")
	gitRun(t, dir, "add", "a")
	if err := Unstage(dir, "a"); err != nil {
		t.Fatalf("Unstage: %v", err)
	}
	if got := strings.TrimSpace(mustOut(t, dir, "status", "--porcelain")); got != "?? a" {
		t.Errorf("status %q", got)
	}
}

func TestCommitStaged_staged(t *testing.T) {
	dir := commitFixture(t)
	write(t, filepath.Join(dir, "a"), "changed")
	write(t, filepath.Join(dir, "b"), "left out")
	if err := Stage(dir, "a"); err != nil {
		t.Fatal(err)
	}

	if _, err := CommitStaged(dir, "feat: change a\n\nWhy it changed.", false); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if got := mustOut(t, dir, "log", "-1", "--format=%B"); strings.TrimSpace(got) != "feat: change a\n\nWhy it changed." {
		t.Errorf("message %q", got)
	}
	if got := strings.TrimSpace(mustOut(t, dir, "status", "--porcelain")); got != "?? b" {
		t.Errorf("an unstaged file was committed: %q", got)
	}
}

func TestCommitStaged_amendKeepsOneCommit(t *testing.T) {
	dir := commitFixture(t)
	write(t, filepath.Join(dir, "a"), "changed")
	if err := Stage(dir, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := CommitStaged(dir, "base, reworded", true); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if n := strings.TrimSpace(mustOut(t, dir, "rev-list", "--count", "HEAD")); n != "1" {
		t.Errorf("%s commits, want the last one amended", n)
	}
	if msg, _ := LastMessage(dir); msg != "base, reworded" {
		t.Errorf("LastMessage %q", msg)
	}
}

func TestCommitStaged_needsAMessage(t *testing.T) {
	dir := commitFixture(t)
	if _, err := CommitStaged(dir, "  \n", false); err == nil {
		t.Fatal("want a refusal for an empty message")
	}
}

func mustOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := Output(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}
