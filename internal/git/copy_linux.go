package git

import "os/exec"

// reflinkCopyCmd copies src to dst with GNU cp, sharing blocks where the
// filesystem supports it.
func reflinkCopyCmd(src, dst string) *exec.Cmd {
	return exec.Command("cp", "-a", "--reflink=auto", src, dst)
}

// osToolDirs is empty: a Linux daemon's PATH already reaches installed tools.
func osToolDirs() []string { return nil }
