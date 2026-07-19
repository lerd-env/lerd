package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/geodro/lerd/internal/feedback"
)

// applyBackground drives the light/dark palette once bubbletea reports the
// terminal background. Flipping to light must invert the greys and the
// on-accent foreground; the default (dark) must be restored for other tests.
func TestApplyBackgroundFlipsAdaptivePalette(t *testing.T) {
	defer applyBackground(true)

	applyBackground(false)
	if onAccent != lipgloss.Color("#f5f5f5") {
		t.Errorf("onAccent on a light terminal = %v, want light #f5f5f5", onAccent)
	}
	if colDivider != lipgloss.Color("#d1d5db") {
		t.Errorf("colDivider on a light terminal = %v, want gray-300 #d1d5db", colDivider)
	}
	if feedback.ColDim != lipgloss.Color("#4b5563") {
		t.Errorf("feedback.ColDim on a light terminal = %v, want gray-600 #4b5563", feedback.ColDim)
	}

	applyBackground(true)
	if onAccent != lipgloss.Color("#0b0b0b") {
		t.Errorf("onAccent on a dark terminal = %v, want dark #0b0b0b", onAccent)
	}
	if colDivider != lipgloss.Color("#374151") {
		t.Errorf("colDivider on a dark terminal = %v, want gray-700 #374151", colDivider)
	}
}

// A BackgroundColorMsg from bubbletea must drive the palette flip through Update.
func TestUpdateHandlesBackgroundColorMsg(t *testing.T) {
	defer applyBackground(true)
	m := NewModel("test")
	m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
	if darkBackground {
		t.Error("a white background should have set darkBackground=false")
	}
}
