//go:build !windows

package filelock

import (
	"os"
	"syscall"
)

func lock(f *os.File, how int) error {
	return syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB)
}

func tryExclusive(f *os.File) error { return lock(f, syscall.LOCK_EX) }

func tryShared(f *os.File) error { return lock(f, syscall.LOCK_SH) }

func unlock(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
