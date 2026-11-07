package git

import (
	"errors"
	"strings"
)

// Stage adds files' changes to the index, deletions included. Names are taken
// literally, so a file called *.txt never stages every text file.
func Stage(dir string, files ...string) error {
	return withPaths(dir, files, "add", "-A")
}

// Unstage takes files' changes back out of the index, leaving the files as they
// are. Before the first commit there is no HEAD to restore from, so they are dropped.
func Unstage(dir string, files ...string) error {
	if _, err := Output(dir, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		return withPaths(dir, files, "rm", "-q", "--cached")
	}
	return withPaths(dir, files, "restore", "--staged")
}

// withPaths feeds the names on stdin, so a section of thousands of untracked
// files does not overflow the command line.
func withPaths(dir string, files []string, args ...string) error {
	if len(files) == 0 {
		return errors.New("no file given")
	}
	args = append([]string{"--literal-pathspecs"}, args...)
	args = append(args, "--pathspec-from-file=-", "--pathspec-file-nul")
	_, err := runQuietIn(dir, strings.Join(files, "\x00"), args...)
	return err
}

// CommitStaged records what is staged, or rewrites the last commit with it when
// amend is set. Hooks run as they would at a terminal, without one to prompt.
func CommitStaged(dir, message string, amend bool) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", errors.New("a commit needs a message")
	}
	args := []string{"commit", "-q", "-m", message}
	if amend {
		args = append(args, "--amend")
	}
	return runQuiet(dir, args...)
}

// LastMessage is the full message of the commit HEAD points at, for amending it.
func LastMessage(dir string) (string, error) {
	out, err := Output(dir, "log", "-1", "--format=%B")
	return strings.TrimSpace(out), err
}
