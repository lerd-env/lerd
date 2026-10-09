//go:build windows

package filelock

import (
	"os"

	"golang.org/x/sys/windows"
)

func lock(f *os.File, flags uint32) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), flags|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, new(windows.Overlapped))
}

func tryExclusive(f *os.File) error { return lock(f, windows.LOCKFILE_EXCLUSIVE_LOCK) }

func tryShared(f *os.File) error { return lock(f, 0) }

func unlock(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
}
