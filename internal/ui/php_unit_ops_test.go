package ui

import (
	"errors"
	"testing"
)

// Starting or stopping a version has to reach whatever is serving it. Under the
// native runtime that is the launchd pool on the host, and acting on the
// container instead did nothing at all to what was actually running.
func TestPHPVersionStartStopFollowTheRuntime(t *testing.T) {
	var ensured, stopped string
	origEnsure, origStop := nativeEnsureFn, nativeStopFn
	nativeEnsureFn = func(v string) error { ensured = v; return nil }
	nativeStopFn = func(v string) error { stopped = v; return nil }
	t.Cleanup(func() { nativeEnsureFn, nativeStopFn = origEnsure, origStop })

	if err := startPHPVersion(true, "8.5"); err != nil {
		t.Fatal(err)
	}
	if ensured != "8.5" {
		t.Errorf("native start reached %q, want the host pool for 8.5", ensured)
	}
	if err := stopPHPVersion(true, "8.5"); err != nil {
		t.Fatal(err)
	}
	if stopped != "8.5" {
		t.Errorf("native stop reached %q, want the host pool for 8.5", stopped)
	}
}

func TestPHPVersionStartSurfacesTheNativeFailure(t *testing.T) {
	origEnsure := nativeEnsureFn
	nativeEnsureFn = func(string) error { return errors.New("no build installed") }
	t.Cleanup(func() { nativeEnsureFn = origEnsure })

	if err := startPHPVersion(true, "8.5"); err == nil {
		t.Error("a pool that will not start must be reported, not swallowed")
	}
}
