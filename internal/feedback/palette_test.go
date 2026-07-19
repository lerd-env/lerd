package feedback

import (
	"testing"

	"charm.land/lipgloss/v2"
)

// The accent, titles, and running status share the terminal's green (ANSI "2")
// so the CLI and TUI follow the user's terminal theme rather than a fixed brand
// red, which users read as an error. Guard against a regression back to a
// hardcoded red accent.
func TestAccentFollowsTerminalTheme(t *testing.T) {
	green := lipgloss.Color("2")
	for name, got := range map[string]any{
		"ColTitle":   ColTitle,
		"ColAccent":  ColAccent,
		"ColRunning": ColRunning,
	} {
		if got != green {
			t.Errorf("%s = %v, want terminal green %v", name, got, green)
		}
	}
	if ColFailing != lipgloss.Color("1") {
		t.Errorf("ColFailing = %v, want terminal red %v", ColFailing, lipgloss.Color("1"))
	}
	if ColAccent == ColFailing {
		t.Error("accent shares the failure colour, so accented UI reads as an error")
	}
}

// The greys can't follow the terminal theme (ANSI has one grey), so they stay
// hex but adapt to a light vs dark terminal. lipgloss v2 dropped AdaptiveColor,
// so adaptivity now lives in the adaptive() helper: the dark value must be
// unchanged from the old fixed palette so a dark terminal looks identical, and a
// light background must invert to the light value.
func TestGreysAdaptToTerminalBackground(t *testing.T) {
	if ColDivider != lipgloss.Color("#374151") {
		t.Errorf("ColDivider = %v, want the unchanged gray-700 on a dark terminal", ColDivider)
	}
	if ColDim != lipgloss.Color("#6b7280") {
		t.Errorf("ColDim = %v, want the unchanged gray-500 on a dark terminal", ColDim)
	}

	prev := darkBackground
	darkBackground = false
	defer func() { darkBackground = prev }()
	if adaptive("#d1d5db", "#374151") != lipgloss.Color("#d1d5db") {
		t.Error("on a light background the grey should invert to the light hex")
	}
}
