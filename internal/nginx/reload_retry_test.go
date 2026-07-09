package nginx

import (
	"errors"
	"testing"
	"time"
)

func TestReloadWithRetry_SucceedsAfterTransientFailures(t *testing.T) {
	n := 0
	err := reloadWithRetry(func() error {
		n++
		if n < 3 {
			return errors.New("cannot load certificate: No such file")
		}
		return nil
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("expected success once the reload settles, got %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 attempts, got %d", n)
	}
}

func TestReloadWithRetry_ReturnsLastErrorOnTimeout(t *testing.T) {
	want := errors.New("persistent config error")
	err := reloadWithRetry(func() error { return want }, 300*time.Millisecond)
	if !errors.Is(err, want) {
		t.Fatalf("expected the last reload error, got %v", err)
	}
}

func TestReloadWithRetry_SucceedsFirstTry(t *testing.T) {
	n := 0
	err := reloadWithRetry(func() error { n++; return nil }, time.Second)
	if err != nil || n != 1 {
		t.Fatalf("expected one successful attempt, got n=%d err=%v", n, err)
	}
}

// A stopped nginx will not come back inside the retry window, so the caller is
// told immediately instead of stalling for the full timeout.
func TestReloadWithRetry_DoesNotRetryStoppedNginx(t *testing.T) {
	n := 0
	start := time.Now()
	err := reloadWithRetry(func() error { n++; return ErrNotRunning }, 5*time.Second)
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("expected ErrNotRunning, got %v", err)
	}
	if n != 1 {
		t.Fatalf("expected a single attempt, got %d", n)
	}
	if time.Since(start) > time.Second {
		t.Fatal("a stopped nginx should fail fast, not burn the retry window")
	}
}

func stubReload(t *testing.T, execErr error, running bool) {
	t.Helper()
	origExec, origRunning := reloadExecFn, containerRunningFn
	reloadExecFn = func() error { return execErr }
	containerRunningFn = func(string) (bool, error) { return running, nil }
	t.Cleanup(func() { reloadExecFn, containerRunningFn = origExec, origRunning })
}

// Reload against a stopped container reports ErrNotRunning rather than the raw
// podman exec failure, so callers can tell "nothing to signal" from "reload
// rejected the config".
func TestReload_StoppedContainerReportsNotRunning(t *testing.T) {
	stubReload(t, errors.New("no such container"), false)

	if err := Reload(); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("expected ErrNotRunning, got %v", err)
	}
}

// A running nginx that rejects the config is a real failure and must surface as
// itself, not be laundered into ErrNotRunning.
func TestReload_RunningContainerSurfacesRealError(t *testing.T) {
	want := errors.New("cannot load certificate")
	stubReload(t, want, true)

	err := Reload()
	if !errors.Is(err, want) {
		t.Fatalf("expected the reload error, got %v", err)
	}
	if errors.Is(err, ErrNotRunning) {
		t.Fatal("a running nginx must not report ErrNotRunning")
	}
}

func TestReload_SuccessSkipsTheRunningCheck(t *testing.T) {
	origExec, origRunning := reloadExecFn, containerRunningFn
	checked := false
	reloadExecFn = func() error { return nil }
	containerRunningFn = func(string) (bool, error) { checked = true; return true, nil }
	t.Cleanup(func() { reloadExecFn, containerRunningFn = origExec, origRunning })

	if err := Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if checked {
		t.Error("a successful reload should not pay for an extra inspect")
	}
}
