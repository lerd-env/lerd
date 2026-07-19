package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// A click while a modal overlay is open must be swallowed: View() stops
// rescanning zones during a modal, so the registered regions are stale and a
// click would otherwise act on whatever was under the cursor in the last base
// frame (switch tabs, move a cursor, or dismiss a half-finished picker).
func TestMouseClick_IgnoredWhileModalOpen(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.activeTab = tabDashboard
	m.width, m.height = 150, 40
	_ = m.render() // register the base-frame zones, including the Services tab

	z := waitZone("tab:" + tabServices.label())
	if z.IsZero() {
		t.Fatalf("services tab zone not registered after render")
	}

	// Open a modal, then click where the Services tab used to be.
	m.paletteActive = true
	msg := tea.MouseClickMsg{X: z.StartX, Y: z.StartY, Button: tea.MouseLeft}
	next, _ := m.Update(msg)
	m = next.(*Model)

	if m.activeTab != tabDashboard {
		t.Fatalf("click under an open modal switched tabs to %d; should have been swallowed", m.activeTab)
	}
	if !m.paletteActive {
		t.Fatalf("click under an open modal closed the modal; should have been swallowed")
	}
}

// A picker open in modal mode must not be dismissed by a stray click, since
// switchTab (reachable from a tab-zone click) calls closePicker().
func TestMouseClick_DoesNotDismissOpenPicker(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.activeTab = tabSites
	m.focus = paneSites
	m.width, m.height = 150, 40
	_ = m.render()

	z := waitZone("tab:" + tabServices.label())
	if z.IsZero() {
		t.Fatalf("services tab zone not registered after render")
	}

	m.pickerKind = kindPHP // open the picker in modal mode
	if !m.modalActive() {
		t.Fatalf("setting pickerKind should make a modal active")
	}
	msg := tea.MouseClickMsg{X: z.StartX, Y: z.StartY, Button: tea.MouseLeft}
	next, _ := m.Update(msg)
	m = next.(*Model)

	if m.pickerKind != kindPHP {
		t.Fatalf("click under an open picker dismissed it (pickerKind=%d)", m.pickerKind)
	}
	if m.activeTab != tabSites {
		t.Fatalf("click under an open picker switched tabs to %d", m.activeTab)
	}
}
