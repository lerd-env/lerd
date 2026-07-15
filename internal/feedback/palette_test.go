package feedback

import "testing"

// The accent, titles, and running status share the terminal's green (ANSI "2")
// so the CLI and TUI follow the user's terminal theme rather than a fixed brand
// red, which users read as an error. Guard against a regression back to a
// hardcoded red accent.
func TestAccentFollowsTerminalTheme(t *testing.T) {
	green := "2"
	for name, got := range map[string]string{
		"ColTitle":   string(ColTitle),
		"ColAccent":  string(ColAccent),
		"ColRunning": string(ColRunning),
	} {
		if got != green {
			t.Errorf("%s = %q, want terminal green %q", name, got, green)
		}
	}
	if string(ColFailing) != "1" {
		t.Errorf("ColFailing = %q, want terminal red %q", string(ColFailing), "1")
	}
	if ColAccent == ColFailing {
		t.Error("accent shares the failure colour, so accented UI reads as an error")
	}
}

// The greys can't follow the terminal theme (ANSI has one grey), so they stay
// hex but adapt to a light vs dark terminal. The dark value must be unchanged
// from the old fixed palette so a dark terminal looks identical, and the light
// value must differ so a light terminal actually inverts.
func TestGreysAdaptToTerminalBackground(t *testing.T) {
	if ColDivider.Dark != "#374151" {
		t.Errorf("ColDivider dark = %q, want the unchanged gray-700 %q", ColDivider.Dark, "#374151")
	}
	if ColDivider.Light == ColDivider.Dark {
		t.Error("ColDivider light and dark are identical, so it does not adapt on a light terminal")
	}
	if ColDim.Dark != "#6b7280" {
		t.Errorf("ColDim dark = %q, want the unchanged gray-500 %q", ColDim.Dark, "#6b7280")
	}
	if ColDim.Light == ColDim.Dark {
		t.Error("ColDim light and dark are identical, so it does not adapt on a light terminal")
	}
}
