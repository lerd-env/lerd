package git

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo creates a fresh git repo in t.TempDir with a single initial
// commit so refs/heads/<defaultBranch> resolves. Returns the repo path and
// the resolved default branch name (init.defaultBranch varies across
// systems, so callers shouldn't assume "main").
func initRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	mustRun(t, dir, "init", "-q")
	// Required for git commit; doesn't pollute the user's config because
	// we set them locally on this repo only.
	mustRun(t, dir, "config", "user.email", "test@example.com")
	mustRun(t, dir, "config", "user.name", "test")
	mustRun(t, dir, "commit", "--allow-empty", "-q", "-m", "init")

	headData, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
	if err != nil {
		t.Fatalf("read HEAD: %v", err)
	}
	const prefix = "ref: refs/heads/"
	line := strings.TrimSpace(string(headData))
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("unexpected HEAD %q", line)
	}
	return dir, strings.TrimPrefix(line, prefix)
}

func mustRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// ── BranchExists ─────────────────────────────────────────────────────────────

func TestBranchExists_trueForCurrentBranch(t *testing.T) {
	dir, branch := initRepo(t)
	if !BranchExists(dir, branch) {
		t.Errorf("expected BranchExists(%s) = true", branch)
	}
}

func TestBranchExists_falseForUnknownBranch(t *testing.T) {
	dir, _ := initRepo(t)
	if BranchExists(dir, "no-such-branch") {
		t.Error("expected BranchExists(no-such-branch) = false")
	}
}

func TestBranchExists_falseForEmptyBranch(t *testing.T) {
	dir, _ := initRepo(t)
	if BranchExists(dir, "") {
		t.Error("expected BranchExists with empty branch = false")
	}
}

func TestBranchExists_falseWhenDirIsNotARepo(t *testing.T) {
	tmp := t.TempDir()
	if BranchExists(tmp, "main") {
		t.Error("expected BranchExists in non-repo = false")
	}
}

// ── Output ────────────────────────────────────────────────────────────────────

func TestOutput_returnsStdout(t *testing.T) {
	dir, _ := initRepo(t)
	out, err := Output(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	sha := strings.TrimSpace(out)
	if len(sha) != 40 {
		t.Errorf("expected 40-char sha, got %q", sha)
	}
}

func TestOutput_returnsErrorOnFailure(t *testing.T) {
	dir, _ := initRepo(t)
	out, err := Output(dir, "show-ref", "--verify", "refs/heads/nope")
	if err == nil {
		t.Fatal("expected error for missing ref")
	}
	if out != "" {
		t.Errorf("expected empty stdout on failure, got %q", out)
	}
}

func TestOutput_emptyDirUsesCwd(t *testing.T) {
	// `git --version` succeeds with or without a working directory, so
	// passing "" should still produce non-empty stdout.
	out, err := Output("", "--version")
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if !strings.Contains(out, "git version") {
		t.Errorf("expected version banner, got %q", out)
	}
}

// ── Run ───────────────────────────────────────────────────────────────────────

func TestRun_writesStdoutAndStderrToLog(t *testing.T) {
	dir, _ := initRepo(t)
	var buf bytes.Buffer
	if err := Run(dir, &buf, "status", "--short"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// `git status --short` on a clean repo prints nothing, but the
	// command should still succeed. Spot-check that the writer was
	// usable (no panic) by running a verbose command instead.
	buf.Reset()
	if err := Run(dir, &buf, "log", "-n1", "--oneline"); err != nil {
		t.Fatalf("Run log: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected git log output to land in the log writer")
	}
}

func TestRun_returnsErrorOnFailure(t *testing.T) {
	dir, _ := initRepo(t)
	err := Run(dir, nil, "show-ref", "--verify", "refs/heads/nope")
	if err == nil {
		t.Fatal("expected error for missing ref")
	}
	if !strings.Contains(err.Error(), "show-ref") {
		t.Errorf("expected wrapped error to mention subcommand, got %q", err)
	}
}

func TestRun_nilLogDiscardsOutput(t *testing.T) {
	dir, _ := initRepo(t)
	// Passing nil must not panic and must still report success/failure
	// based on the exit code.
	if err := Run(dir, nil, "log", "-n1", "--oneline"); err != nil {
		t.Errorf("expected success with nil log, got %v", err)
	}
}

// ── RunTTY ────────────────────────────────────────────────────────────────────

func TestRunTTY_succeedsForKnownGoodCommand(t *testing.T) {
	dir, _ := initRepo(t)
	if err := RunTTY(dir, "status", "--short"); err != nil {
		t.Errorf("RunTTY: %v", err)
	}
}

func TestRunTTY_returnsErrorOnFailure(t *testing.T) {
	dir, _ := initRepo(t)
	err := RunTTY(dir, "show-ref", "--verify", "refs/heads/nope")
	if err == nil {
		t.Fatal("expected error for missing ref")
	}
}

// ── RunCaptureStderr ──────────────────────────────────────────────────────────

func TestRunCaptureStderr_capturesGitErrorMessage(t *testing.T) {
	dir, _ := initRepo(t)
	stderr, err := RunCaptureStderr(dir, "worktree", "remove", "no-such-path")
	if err == nil {
		t.Fatal("expected error removing nonexistent worktree")
	}
	if stderr == "" {
		t.Error("expected captured stderr to be non-empty on failure")
	}
}

func TestRunCaptureStderr_emptyStderrOnSuccess(t *testing.T) {
	dir, _ := initRepo(t)
	stderr, err := RunCaptureStderr(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("RunCaptureStderr: %v", err)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Errorf("expected empty stderr on success, got %q", stderr)
	}
}
