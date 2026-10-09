package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Fetch updates the checkout's remote-tracking refs so ahead/behind is current.
func Fetch(dir string) (string, error) { return runQuiet(dir, "fetch") }

// Pull fast-forwards the checkout to its upstream. Anything that would need a
// merge is refused by git, so the tree is never left mid-merge.
func Pull(dir string) (string, error) { return runQuiet(dir, "pull", "--ff-only") }

// Push sends the branch to its upstream, never forced: a remote that moved on
// makes git reject it. The explicit refspec keeps push.default and configured
// push refspecs, which can match every branch or force, out of it. A branch
// with no upstream is published to origin under its own name and tracks it.
func Push(dir string) (string, error) {
	branch, err := Output(dir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", errors.New("not on a branch")
	}
	branch = strings.TrimSpace(branch)
	remote, _ := Output(dir, "config", "branch."+branch+".remote")
	merge, _ := Output(dir, "config", "branch."+branch+".merge")
	if strings.TrimSpace(remote) == "" || strings.TrimSpace(merge) == "" {
		if !hasOrigin(dir) {
			return "", fmt.Errorf("%s has no upstream to push to", branch)
		}
		ref := "refs/heads/" + branch
		return runQuiet(dir, "push", "--set-upstream", "origin", ref+":"+ref)
	}
	// The branch by name, not HEAD: a switch between these calls must not redirect the push.
	return runQuiet(dir, "push", strings.TrimSpace(remote), "refs/heads/"+branch+":"+strings.TrimSpace(merge))
}

// Switch checks out branch in the checkout at dir, never forced: changes the
// other branch would overwrite make git refuse. A remote-tracking ref such as
// origin/x becomes a local branch x that tracks it.
func Switch(dir, branch string) (string, error) {
	if !BranchExists(dir, branch) {
		if _, err := Output(dir, "show-ref", "--verify", "--quiet", "refs/remotes/"+branch); err == nil {
			return runQuiet(dir, "switch", "--track", branch)
		}
	}
	return runQuiet(dir, "switch", branch)
}

// SwitchNew creates branch at base (HEAD when empty) and checks it out. It
// tracks nothing, so a first push does not land on the base's upstream.
func SwitchNew(dir, branch, base string) (string, error) {
	args := []string{"switch", "--no-track", "-c", branch}
	if base != "" {
		args = append(args, base)
	}
	return runQuiet(dir, args...)
}

// runQuiet runs a git command for the dashboard and returns its output,
// or an error carrying git's own words. Prompts are off: nobody is at a
// terminal to answer them, and a credential prompt would hang the request.
func runQuiet(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	msg := withoutHints(string(out))
	if err != nil {
		if msg == "" {
			return "", err
		}
		return "", errors.New(msg)
	}
	return msg, nil
}

// withoutHints drops git's "hint:" advice, which suggests terminal commands
// and buries the one line that says what went wrong.
func withoutHints(out string) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "hint:") {
			keep = append(keep, line)
		}
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}
