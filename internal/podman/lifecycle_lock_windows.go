//go:build windows

package podman

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"

	"github.com/geodro/lerd/internal/config"
)

func lifecycleLockFile() string {
	return filepath.Join(config.DataDir(), "lifecycle.lock")
}

// LockLifecycle holds lerd's start/stop lock until release is called, waiting
// for any start or stop already holding it.
func LockLifecycle() (func(), error) {
	release, _, err := lockLifecycle(windows.LOCKFILE_EXCLUSIVE_LOCK)
	return release, err
}

// TryLockLifecycle takes the lifecycle lock only if no start or stop holds it,
// reporting false instead of waiting when one does.
func TryLockLifecycle() (func(), bool, error) {
	return lockLifecycle(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
}

func lockLifecycle(flags uint32) (func(), bool, error) {
	if err := os.MkdirAll(config.DataDir(), 0755); err != nil {
		return nil, false, err
	}
	f, err := os.OpenFile(lifecycleLockFile(), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, false, err
	}
	h := windows.Handle(f.Fd())
	if err := windows.LockFileEx(h, flags, 0, 1, 0, new(windows.Overlapped)); err != nil {
		_ = f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() {
		_ = windows.UnlockFileEx(h, 0, 1, 0, new(windows.Overlapped))
		_ = f.Close()
	}, true, nil
}

// LifecycleInFlight reports whether a lerd start or stop holds the lifecycle
// lock, probing with a shared non-blocking lock so it never waits on the holder.
func LifecycleInFlight() bool {
	f, err := os.Open(lifecycleLockFile())
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck
	h := windows.Handle(f.Fd())
	if err := windows.LockFileEx(h, windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, new(windows.Overlapped)); err != nil {
		return true
	}
	_ = windows.UnlockFileEx(h, 0, 1, 0, new(windows.Overlapped))
	return false
}
