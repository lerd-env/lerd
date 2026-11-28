package main

import (
	"testing"
	"time"

	gitpkg "github.com/geodro/lerd/internal/git"
)

func stubWorktreeDeps(t *testing.T) *int {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	calls := 0
	prev := ensureWorktreeDepsFn
	ensureWorktreeDepsFn = func(string, string, string, bool) { calls++ }
	t.Cleanup(func() { ensureWorktreeDepsFn = prev })
	return &calls
}

func TestInstallWorktreeDeps_installsWhenNobodyElseIs(t *testing.T) {
	calls := stubWorktreeDeps(t)
	if !installWorktreeDeps(t.TempDir(), t.TempDir(), "b.site.test", false) {
		t.Fatal("reported another install with the lock free")
	}
	if *calls != 1 {
		t.Fatalf("installed %d times, want 1", *calls)
	}
}

// The watcher's event loop calls this, so a held lock must be reported at once,
// never waited on there.
func TestInstallWorktreeDeps_reportsAHeldLockWithoutWaiting(t *testing.T) {
	calls := stubWorktreeDeps(t)
	wt := t.TempDir()
	release, err := gitpkg.LockInstall(wt, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	start := time.Now()
	if installWorktreeDeps(t.TempDir(), wt, "b.site.test", false) {
		t.Fatal("reported an install of its own while another held the lock")
	}
	if waited := time.Since(start); waited > 200*time.Millisecond {
		t.Errorf("waited %s on the held lock", waited)
	}
	if *calls != 0 {
		t.Errorf("installed %d times on top of the running install, want 0", *calls)
	}
}

// A new worktree's servers start only once the install holding its lock is done.
func TestWaitForWorktreeInstall_returnsOnceTheHolderIsDone(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	wt := t.TempDir()
	release, err := gitpkg.LockInstall(wt, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { waitForWorktreeInstall(wt); close(done) }()
	select {
	case <-done:
		t.Fatal("returned while the install still held the lock")
	case <-time.After(300 * time.Millisecond):
	}
	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("still waiting after the install finished")
	}
}
