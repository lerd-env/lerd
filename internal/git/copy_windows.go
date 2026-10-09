package git

import (
	"errors"
	"os/exec"
)

// reflinkCopyCmd has no cp to run on Windows, so it fails at once and
// CopyTree falls back to the plain Go copy.
func reflinkCopyCmd(string, string) *exec.Cmd {
	return &exec.Cmd{Err: errors.New("reflink copy unsupported on windows")}
}

// osToolDirs is empty: Windows has no Homebrew prefixes to search.
func osToolDirs() []string { return nil }
