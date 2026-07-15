//go:build !nogui

package tray

import (
	"bytes"
	"testing"
)

func TestPickColorIcon(t *testing.T) {
	tests := []struct {
		name         string
		running      bool
		light        bool
		highContrast bool
		wantKind     iconKind
		wantIcon     []byte
	}{
		{"stopped dark panel", false, false, false, iconKindStopped, iconPNG},
		{"stopped light panel", false, true, false, iconKindStopped, iconPNG},
		{"running dark panel", true, false, false, iconKindRunningDark, iconWhitePNG},
		{"running light panel", true, true, false, iconKindRunningLight, iconDarkPNG},
		{"high-contrast running dark panel", true, false, true, iconKindRunningHiC, iconGreenPNG},
		{"high-contrast running light panel", true, true, true, iconKindRunningHiC, iconGreenPNG},
		{"high-contrast stopped stays red", false, true, true, iconKindStopped, iconPNG},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kind, icon := pickColorIcon(tc.running, tc.light, tc.highContrast)
			if kind != tc.wantKind {
				t.Errorf("kind = %d, want %d", kind, tc.wantKind)
			}
			if !bytes.Equal(icon, tc.wantIcon) {
				t.Errorf("icon mismatch for %s", tc.name)
			}
		})
	}
}

// The dark running icon must be a distinct asset from the white one, otherwise
// the light-panel fix is a no-op.
func TestRunningIconsDiffer(t *testing.T) {
	if len(iconDarkPNG) == 0 {
		t.Fatal("iconDarkPNG is empty")
	}
	if bytes.Equal(iconDarkPNG, iconWhitePNG) {
		t.Error("dark and white running icons are identical")
	}
	if len(iconGreenPNG) == 0 {
		t.Fatal("iconGreenPNG is empty")
	}
	if bytes.Equal(iconGreenPNG, iconPNG) {
		t.Error("green running icon is identical to the red stopped icon")
	}
}

// setHighContrast must swap the running icon to green regardless of the panel's
// light/dark preference, and clearing it must fall back to the theme-adaptive
// icon, proving the opt-in takes effect live.
func TestIconStateHighContrastSwitch(t *testing.T) {
	var applied [][]byte
	s := newIconState()
	s.apply = func(icon []byte) { applied = append(applied, icon) }

	s.setRunning(true)
	s.setLight(true)
	s.setHighContrast(true)
	if s.last != iconKindRunningHiC || !bytes.Equal(applied[len(applied)-1], iconGreenPNG) {
		t.Fatal("high-contrast on should show the green icon even on a light panel")
	}
	s.setHighContrast(false)
	if s.last != iconKindRunningLight || !bytes.Equal(applied[len(applied)-1], iconDarkPNG) {
		t.Error("high-contrast off should restore the theme-adaptive icon")
	}
}

// setLight must flip the running icon between the white and dark variants
// without a running-state change, proving live theme switches take effect.
func TestIconStateLightSwitch(t *testing.T) {
	var applied [][]byte
	s := newIconState()
	s.apply = func(icon []byte) { applied = append(applied, icon) }

	s.setRunning(true)
	if !bytes.Equal(applied[len(applied)-1], iconWhitePNG) {
		t.Fatal("running on default dark panel should show the white icon")
	}
	s.setLight(true)
	if s.last != iconKindRunningLight || !bytes.Equal(applied[len(applied)-1], iconDarkPNG) {
		t.Error("light panel should swap the running icon to dark")
	}
	s.setLight(false)
	if s.last != iconKindRunningDark || !bytes.Equal(applied[len(applied)-1], iconWhitePNG) {
		t.Error("back to dark panel should restore the white icon")
	}
}

// Redundant updates must not re-issue SetIcon, so the systray isn't thrashed on
// every 5s poll tick.
func TestIconStateSkipsRedundant(t *testing.T) {
	var calls int
	s := newIconState()
	s.apply = func([]byte) { calls++ }

	s.setRunning(true)
	s.setRunning(true)
	s.setLight(false)
	if calls != 1 {
		t.Errorf("apply called %d times, want 1", calls)
	}
}
