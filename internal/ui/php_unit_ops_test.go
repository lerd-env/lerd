package ui

import (
	"errors"
	"testing"
)

// stubNativePHP swaps the native seams so a test never touches launchd or the
// real xdebug ini, and records what reached each one, in order.
func stubNativePHP(t *testing.T, ensure func(string) error) *[]string {
	t.Helper()
	var calls []string
	origEnsure, origStop, origIni := nativeEnsureFn, nativeStopFn, ensureXdebugIniFn
	nativeEnsureFn = func(v string) error { calls = append(calls, "ensure "+v); return ensure(v) }
	nativeStopFn = func(v string) error { calls = append(calls, "stop "+v); return nil }
	ensureXdebugIniFn = func(v string) error { calls = append(calls, "ini "+v); return nil }
	t.Cleanup(func() { nativeEnsureFn, nativeStopFn, ensureXdebugIniFn = origEnsure, origStop, origIni })
	return &calls
}

// Starting or stopping a version has to reach whatever is serving it. Under the
// native runtime that is the launchd pool on the host, and acting on the
// container instead did nothing at all to what was actually running.
func TestPHPVersionStartStopFollowTheRuntime(t *testing.T) {
	calls := stubNativePHP(t, func(string) error { return nil })

	if err := startPHPVersion(true, "8.5"); err != nil {
		t.Fatal(err)
	}
	if err := stopPHPVersion(true, "8.5"); err != nil {
		t.Fatal(err)
	}
	// The shared ini is migrated before the overrides are written, so an ini
	// left from an older lerd can't load xdebug a second time.
	want := []string{"ini 8.5", "ensure 8.5", "stop 8.5"}
	if len(*calls) != len(want) {
		t.Fatalf("calls = %v, want %v", *calls, want)
	}
	for i := range want {
		if (*calls)[i] != want[i] {
			t.Fatalf("calls = %v, want %v", *calls, want)
		}
	}
}

func TestPHPVersionStartSurfacesTheNativeFailure(t *testing.T) {
	stubNativePHP(t, func(string) error { return errors.New("no build installed") })

	if err := startPHPVersion(true, "8.5"); err == nil {
		t.Error("a pool that will not start must be reported, not swallowed")
	}
}
