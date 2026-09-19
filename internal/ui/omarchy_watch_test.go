package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitForSignal drains one change off the channel, or fails. The watcher runs
// on real filesystem events, so the test waits on the signal rather than on a
// fixed sleep.
func waitForSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("no theme change reported within 5s")
	}
}

func TestWatchOmarchyThemeReportsAThemeSwap(t *testing.T) {
	state := t.TempDir()
	current := filepath.Join(state, "omarchy", "current")
	if err := os.MkdirAll(filepath.Join(current, "theme"), 0755); err != nil {
		t.Fatal(err)
	}

	changed := make(chan struct{}, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := watchOmarchyTheme(ctx, current, 20*time.Millisecond, func() { changed <- struct{}{} }); err != nil {
		t.Fatalf("watchOmarchyTheme: %v", err)
	}

	// What omarchy-theme-set does: stage the new theme beside the old one, then
	// move it into place. The directory is replaced, never rewritten in place.
	next := filepath.Join(current, "next-theme")
	if err := os.MkdirAll(next, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(current, "theme")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(next, filepath.Join(current, "theme")); err != nil {
		t.Fatal(err)
	}
	waitForSignal(t, changed)
}

func TestWatchOmarchyThemeReportsANameChange(t *testing.T) {
	state := t.TempDir()
	current := filepath.Join(state, "omarchy", "current")
	if err := os.MkdirAll(filepath.Join(current, "theme"), 0755); err != nil {
		t.Fatal(err)
	}

	changed := make(chan struct{}, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := watchOmarchyTheme(ctx, current, 20*time.Millisecond, func() { changed <- struct{}{} }); err != nil {
		t.Fatalf("watchOmarchyTheme: %v", err)
	}

	if err := os.WriteFile(filepath.Join(current, "theme.name"), []byte("nord\n"), 0644); err != nil {
		t.Fatal(err)
	}
	waitForSignal(t, changed)
}

// A swap fires a burst of events. The dashboard should be told once, or every
// open tab refetches the theme list several times for one keystroke.
func TestWatchOmarchyThemeCollapsesABurst(t *testing.T) {
	state := t.TempDir()
	current := filepath.Join(state, "omarchy", "current")
	if err := os.MkdirAll(current, 0755); err != nil {
		t.Fatal(err)
	}

	changed := make(chan struct{}, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := watchOmarchyTheme(ctx, current, 150*time.Millisecond, func() { changed <- struct{}{} }); err != nil {
		t.Fatalf("watchOmarchyTheme: %v", err)
	}

	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filepath.Join(current, "theme.name"), []byte("nord\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	waitForSignal(t, changed)
	select {
	case <-changed:
		t.Error("a burst of events reported more than one change")
	case <-time.After(400 * time.Millisecond):
	}
}

// Nothing to watch is the ordinary case on a machine without Omarchy, and it
// must not take the daemon down with it.
func TestWatchOmarchyThemeWithoutOmarchy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := watchOmarchyTheme(ctx, filepath.Join(t.TempDir(), "absent"), time.Millisecond, func() {})
	if err == nil {
		t.Error("watchOmarchyTheme on a missing directory = nil, want an error the caller can ignore")
	}
}
