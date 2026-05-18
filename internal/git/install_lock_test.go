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
