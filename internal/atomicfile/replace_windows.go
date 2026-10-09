//go:build windows

package atomicfile

import (
	"errors"
	"os"
	"syscall"
	"time"
)

const (
	errorAccessDenied     syscall.Errno = 5
	errorSharingViolation syscall.Errno = 32
)

// replaceFile moves tmp over path. Windows refuses to replace a file another
// handle has open without FILE_SHARE_DELETE, which Go's os.Open does not set,
// so a reader mid-read makes the rename fail for a moment. That is transient,
// so it is retried briefly before giving up.
func replaceFile(tmp, path string) error {
	var err error
	for i := 0; i < 40; i++ {
		if err = os.Rename(tmp, path); err == nil {
			return nil
		}
		if !errors.Is(err, errorAccessDenied) && !errors.Is(err, errorSharingViolation) {
			return err
		}
		time.Sleep(time.Duration(i+1) * 2 * time.Millisecond)
	}
	return err
}

// syncDir is a no-op: NTFS journals the metadata of a completed rename, and a
// directory handle cannot be fsynced (FlushFileBuffers fails with access denied).
func syncDir(string) error { return nil }
