package git

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Output runs `git <args>` in dir and returns its stdout as a string. If
// dir is empty, the command inherits the caller's working directory.
// Stderr is discarded; callers that need it should reach for RunCaptureStderr.
func Output(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Run runs `git <args>` in dir, sending stdout and stderr to log (a single
// writer; matches the modal-log pattern used by the worktree flows). When
// log is nil, output is discarded. Empty dir means inherit cwd. The returned
// error wraps the exec failure with the first non-flag arg so callers can
// surface "git worktree: exit status 1" rather than the full arg list.
func Run(dir string, log io.Writer, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if log != nil {
		cmd.Stdout = log
		cmd.Stderr = log
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", argSummary(args), err)
	}
	return nil
}

// RunTTY runs `git <args>` in dir with stdout and stderr wired straight to
// os.Stdout and os.Stderr so the user's shell redirections (`2>/dev/null`,
// pipes, etc) still work. Empty dir means inherit cwd.
func RunTTY(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", argSummary(args), err)
	}
	return nil
}

// RunCaptureStderr runs `git <args>` in dir, mirroring stdout to os.Stdout
// and stderr to both os.Stderr and an internal buffer. The captured stderr
// is returned alongside any exec error so callers can detect specific git
// error messages (e.g. the "use --force" hint) without dropping the live
// stream the user is watching.
func RunCaptureStderr(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	buf := &strings.Builder{}
	cmd.Stderr = io.MultiWriter(os.Stderr, buf)
	err := cmd.Run()
	return buf.String(), err
}

// BranchExists reports whether refs/heads/<branch> resolves inside the repo
// at dir. Returns false when branch is empty, when dir is not a repo, or
// when the ref cannot be resolved.
func BranchExists(dir, branch string) bool {
	if branch == "" {
		return false
	}
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.Run() == nil
}

// argSummary picks the first non-flag arg so error messages read as
// "git worktree: ..." rather than the full command line. Keeps wrap text
// short while still naming the subcommand.
func argSummary(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	if len(args) > 0 {
		return args[0]
	}
	return "(no args)"
}
