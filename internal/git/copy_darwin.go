package git

import "os/exec"

// reflinkCopyCmd copies src to dst with BSD cp's clonefile on APFS.
func reflinkCopyCmd(src, dst string) *exec.Cmd {
	return exec.Command("cp", "-Rc", src, dst)
}

// osToolDirs are Homebrew's two prefixes, Apple Silicon then Intel, which
// launchd leaves off a daemon's PATH.
func osToolDirs() []string { return []string{"/opt/homebrew/bin", "/usr/local/bin"} }
