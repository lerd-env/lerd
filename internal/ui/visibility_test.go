package ui

import (
	"testing"
	"time"
)

func TestNoteVisibilityCounter(t *testing.T) {
	// Reset global state before and after test so other tests aren't affected.
	visibleClients.Store(0)
	t.Cleanup(func() { visibleClients.Store(0) })

	noteVisibility(true) // 1
	noteVisibility(true) // 2
	if v := visibleClients.Load(); v != 2 {
		t.Errorf("expected 2 visible clients, got %d", v)
	}

	noteVisibility(false) // 1 — should NOT change interval to idle yet
	if v := visibleClients.Load(); v != 1 {
		t.Errorf("expected 1 visible client after one decrement, got %d", v)
	}

	noteVisibility(false) // 0 — now idle
	if v := visibleClients.Load(); v != 0 {
		t.Errorf("expected 0 visible clients, got %d", v)
	}

	// Extra decrement (simulates the double-decrement bug we fixed: browser
	// sends visible=false then connection closes) must not go negative.
	noteVisibility(false)
	if v := visibleClients.Load(); v != 0 {
		t.Errorf("counter should not go below 0, got %d", v)
	}
}

func TestNoteVisibilityMultipleConnections(t *testing.T) {
	visibleClients.Store(0)
	t.Cleanup(func() { visibleClients.Store(0) })

	// Simulate two connections: A visible, B visible.
	noteVisibility(true) // A connects (assumed visible) → 1
	noteVisibility(true) // B connects → 2

	// A hides.
	noteVisibility(false) // → 1 (B still visible, should stay at focused interval)
	if v := visibleClients.Load(); v != 1 {
		t.Errorf("expected 1 after A hides, got %d", v)
	}

	// A disconnects — with the fix, since connVisible is false for A, no
	// extra decrement. We simulate the correct behaviour: no call.
	// (The fixed handleWS only calls noteVisibility(false) on disconnect
	// if the connection was still visible.)
	// Counter should still be 1.
	if v := visibleClients.Load(); v != 1 {
		t.Errorf("disconnect of already-hidden A should not change counter; got %d", v)
	}

	// B disconnects while still visible → 0.
	noteVisibility(false) // → 0
	if v := visibleClients.Load(); v != 0 {
		t.Errorf("expected 0 after B disconnects, got %d", v)
	}
}

// TestFocusLeaseExpires is the bug this replaced a flag to fix: a window that
// claimed focus and then went quiet, a second tab, a page that reconnected
// after a restart, kept every desktop notification suppressed until its socket
// was reaped a minute and a quarter later.
func TestFocusLeaseExpires(t *testing.T) {
	resetFocus(t)

	noteFocus(1, true)
	if !uiWindowFocused() {
		t.Fatal("a window that just claimed focus must count")
	}
	// Reach into the lease rather than wait out the TTL.
	focusMu.Lock()
	focusLeases[1] = time.Now().Add(-time.Second)
	focusMu.Unlock()
	if uiWindowFocused() {
		t.Error("a claim nobody renewed must run out")
	}
}

// TestFocusLeaseRenewalKeepsIt checks the window that is actually focused
// holds on to it, since it says so again every few seconds.
func TestFocusLeaseRenewalKeepsIt(t *testing.T) {
	resetFocus(t)

	noteFocus(1, true)
	focusMu.Lock()
	focusLeases[1] = time.Now().Add(-time.Second)
	focusMu.Unlock()
	noteFocus(1, true)
	if !uiWindowFocused() {
		t.Error("a renewed claim must keep the window counted")
	}
}

// TestFocusClaimsAreSeparatePerConnection covers the shape that broke it: one
// page blurring must not release another's claim, and a connection going away
// must release its own.
func TestFocusClaimsAreSeparatePerConnection(t *testing.T) {
	resetFocus(t)

	noteFocus(1, true)
	noteFocus(2, true)
	noteFocus(1, false)
	if !uiWindowFocused() {
		t.Error("the other window still has focus")
	}
	dropFocus(2)
	if uiWindowFocused() {
		t.Error("the last claim went away with its connection")
	}
}

func resetFocus(t *testing.T) {
	t.Helper()
	clear := func() {
		focusMu.Lock()
		focusLeases = map[uint64]time.Time{}
		focusMu.Unlock()
	}
	clear()
	t.Cleanup(clear)
}
