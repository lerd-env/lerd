package git

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Cross-process flock is per-FD on Linux, so two goroutines in the same
// process that both OpenFile the lock path get independent FDs and both
// acquire. The in-process mutex layer fixes that — TryLockInstall must
// refuse a second caller while the first is still holding the lock.
func TestTryLockInstall_serializesInProcess(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()

	release1, ok, err := TryLockInstall(dir)
	if err != nil || !ok {
		t.Fatalf("first try: ok=%v err=%v, want ok=true", ok, err)
	}
	defer release1()

	_, ok2, err := TryLockInstall(dir)
	if err != nil {
		t.Fatalf("second try: unexpected err %v", err)
	}
	if ok2 {
		t.Fatal("second TryLockInstall must return false while first goroutine still holds the lock")
	}
}

// LockInstall blocks rather than racing; concurrent calls must observe
// non-overlapping critical sections.
func TestLockInstall_serializesInProcess(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()

	var holders atomic.Int32
	var maxHolders atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := LockInstall(dir, 5*time.Second)
			if err != nil {
				t.Errorf("LockInstall: %v", err)
				return
			}
			n := holders.Add(1)
			if n > maxHolders.Load() {
				maxHolders.Store(n)
			}
			time.Sleep(10 * time.Millisecond)
			holders.Add(-1)
			release()
		}()
	}
	wg.Wait()
	if got := maxHolders.Load(); got != 1 {
		t.Errorf("max concurrent holders = %d, want 1", got)
	}
}

// InstallInFlight is the signal external tools need in place of guessing from
// directory contents. It must report a running install, go quiet once that
// install finishes, and never be fooled by the lock file left behind, which is
// exactly the trap that makes existence and pid checks useless.
func TestInstallInFlight(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()

	if held, err := InstallInFlight(dir); err != nil || held {
		t.Fatalf("before any install: held=%v err=%v, want held=false", held, err)
	}

	release, err := LockInstall(dir, 5*time.Second)
	if err != nil {
		t.Fatalf("LockInstall: %v", err)
	}
	held, err := InstallInFlight(dir)
	if err != nil {
		t.Fatalf("InstallInFlight while lock held: %v", err)
	}
	if !held {
		t.Error("must report in flight while an installer holds the lock")
	}
	release()

	if held, err := InstallInFlight(dir); err != nil || held {
		t.Errorf("after release, with the lock file still on disk: held=%v err=%v, want held=false", held, err)
	}
}

// The probe must not disturb a real installer: it may not take the exclusive
// lock, so an install starting immediately after a probe still acquires at once.
func TestInstallInFlight_leavesLockAcquirable(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()

	release, err := LockInstall(dir, 5*time.Second)
	if err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	release()

	if _, err := InstallInFlight(dir); err != nil {
		t.Fatalf("InstallInFlight: %v", err)
	}
	release2, err := LockInstall(dir, 2*time.Second)
	if err != nil {
		t.Fatalf("an installer must still acquire right after a probe: %v", err)
	}
	release2()
}
