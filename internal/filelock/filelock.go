// Package filelock wraps advisory whole-file locks so callers stay portable:
// flock on Unix, LockFileEx on Windows. All acquisitions are non-blocking.
package filelock

import "os"

// TryExclusive takes an exclusive lock on f, returning an error when another
// holder has it.
func TryExclusive(f *os.File) error { return tryExclusive(f) }

// TryShared takes a shared lock on f, returning an error when an exclusive
// holder has it.
func TryShared(f *os.File) error { return tryShared(f) }

// Unlock releases a lock taken by TryExclusive or TryShared.
func Unlock(f *os.File) error { return unlock(f) }
