//go:build !windows

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

func releaseArchiveName(ver, arch string) string {
	return fmt.Sprintf("lerd_%s_%s_%s.tar.gz", ver, runtime.GOOS, arch)
}

func extractReleaseArchive(archive, dir string) error {
	cmd := exec.Command("tar", "--no-same-owner", "-xzf", archive, "-C", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}
	return nil
}

// swapBinary atomically replaces dest with a copy of src.
func swapBinary(src, dest string) error {
	tmp := dest + ".tmp"
	if err := copyFile(src, tmp, 0755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
