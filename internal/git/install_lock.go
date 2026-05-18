package git

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// inProcessLocks layers a per-path mutex above flock because flock is
// per-FD on Linux: two goroutines in the same process that both OpenFile
// the lock path get independent FDs and would both acquire.
var inProcessLocks sync.Map

// installLockFile returns a stable lock path for worktreePath under
// <DataDir>/install-locks/<sha1(worktreePath)>.lock. The hash keeps the
// filename short and filesystem-safe regardless of how deeply nested the
// worktree lives. Both the UI process and the watcher daemon resolve the
// same path from the absolute worktree path, so flock serialises across
// processes.
func installLockFile(worktreePath string) (string, error) {
	abs, err := filepath.Abs(worktreePath)
	if err != nil {
		return "", err
	}
	sum := sha1.Sum([]byte(abs))
	dir := filepath.Join(config.DataDir(), "install-locks")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".lock"), nil
}

// procMutexFor returns the singleton per-path mutex for serialising in-
// process callers before they contend for the OS flock.
func procMutexFor(path string) *sync.Mutex {
	v, _ := inProcessLocks.LoadOrStore(path, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// LockInstall acquires an exclusive cross-process lock guarding the
// composer/npm install for worktreePath, blocking until either the lock
// is held or timeout elapses. Returns release() which drops both the
// in-process and OS locks; safe to defer.
func LockInstall(worktreePath string, timeout time.Duration) (func(), error) {
	path, err := installLockFile(worktreePath)
	if err != nil {
		return nil, err
	}
	mu := procMutexFor(path)
	deadline := time.Now().Add(timeout)
	if !lockWithDeadline(mu, deadline) {
		return nil, fmt.Errorf("install lock for %s held by another goroutine", worktreePath)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		mu.Unlock()
		return nil, err
	}
	for {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			_ = f.Truncate(0)
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				_ = f.Close()
				mu.Unlock()
			}, nil
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			mu.Unlock()
			return nil, fmt.Errorf("install lock for %s held by another process", worktreePath)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// TryLockInstall is the non-blocking variant of LockInstall: returns
// (release, true) when the lock was acquired immediately, or (nil, false)
// when another caller holds it. err is non-nil only on filesystem
// failures; "already locked" is reported via the boolean.
func TryLockInstall(worktreePath string) (func(), bool, error) {
	path, err := installLockFile(worktreePath)
	if err != nil {
		return nil, false, err
	}
	mu := procMutexFor(path)
	if !mu.TryLock() {
		return nil, false, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		mu.Unlock()
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		mu.Unlock()
		return nil, false, nil
	}
	_ = f.Truncate(0)
	_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		mu.Unlock()
	}, true, nil
}

// lockWithDeadline polls TryLock until success or deadline.
func lockWithDeadline(mu *sync.Mutex, deadline time.Time) bool {
	for {
		if mu.TryLock() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}
