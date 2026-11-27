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
	installWorktreeDeps(t.TempDir(), t.TempDir(), "b.site.test", false)
	if *calls != 1 {
		t.Fatalf("installed %d times, want 1", *calls)
	}
}

// While lerd worktree add installs, the worktree's dev server must not start on
// a half-filled node_modules, so the watcher waits for that install to end.
func TestInstallWorktreeDeps_waitsForAnInstallAlreadyRunning(t *testing.T) {
	calls := stubWorktreeDeps(t)
	wt := t.TempDir()
	release, err := gitpkg.LockInstall(wt, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { installWorktreeDeps(t.TempDir(), wt, "b.site.test", false); close(done) }()
	select {
	case <-done:
		t.Fatal("returned while another install still held the lock")
	case <-time.After(300 * time.Millisecond):
	}
	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("still waiting after the other install finished")
	}
	if *calls != 0 {
		t.Errorf("installed %d times on top of the running install, want 0", *calls)
	}
}
