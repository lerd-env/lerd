package podman

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/geodro/lerd/internal/config"
)

func lifecycleLockFile() string {
	return filepath.Join(config.DataDir(), "lifecycle.lock")
}

// LockLifecycle holds lerd's start/stop lock until release is called, waiting
// for any start or stop already holding it. A shim run meanwhile (a worker's
// `lerd php`) sees it through LifecycleInFlight and leaves the machine alone.
func LockLifecycle() (func(), error) {
	if err := os.MkdirAll(config.DataDir(), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lifecycleLockFile(), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// LifecycleInFlight reports whether a lerd start or stop holds the lifecycle
// lock. It probes with a shared non-blocking flock, so it never waits on the
// holder; a missing lock file means nothing has ever held it.
func LifecycleInFlight() bool {
	f, err := os.Open(lifecycleLockFile())
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		return true
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false
}
