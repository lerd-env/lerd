//go:build !windows

package atomicfile

import "os"

// replaceFile moves tmp over path. POSIX rename replaces atomically even while
// a reader has the target open.
func replaceFile(tmp, path string) error { return os.Rename(tmp, path) }

// syncDir flushes the directory entry so a crash right after the rename can't
// lose the published file even though its bytes were already fsynced.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
