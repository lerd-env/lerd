//go:build !nogui

package tray

import (
	"bytes"
	"testing"

	"github.com/geodro/lerd/internal/config"
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
		{"stopped dark panel", false, false, false, iconKindStoppedDark, iconOffWhitePNG},
		{"stopped light panel", false, true, false, iconKindStoppedLight, iconOffDarkPNG},
		{"running dark panel", true, false, false, iconKindRunningDark, iconWhitePNG},
		{"running light panel", true, true, false, iconKindRunningLight, iconDarkPNG},
		{"high-contrast running dark panel", true, false, true, iconKindRunningHiC, iconGreenPNG},
		{"high-contrast running light panel", true, true, true, iconKindRunningHiC, iconGreenPNG},
		{"high-contrast stopped is red on any panel", false, true, true, iconKindStoppedHiC, iconOffRedPNG},
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

// Every state must have its own artwork, otherwise a state change shows nothing.
func TestTrayIconsAreDistinct(t *testing.T) {
	icons := map[string][]byte{
		"white": iconWhitePNG, "dark": iconDarkPNG, "green": iconGreenPNG,
		"off-white": iconOffWhitePNG, "off-dark": iconOffDarkPNG, "off-red": iconOffRedPNG,
		"mono": iconMonoPNG, "mono-off": iconMonoOffPNG,
	}
	seen := map[string]string{}
	for name, icon := range icons {
		if len(icon) == 0 {
			t.Fatalf("%s icon is empty", name)
		}
		if other, dup := seen[string(icon)]; dup {
			t.Errorf("%s and %s icons are identical", name, other)
		}
		seen[string(icon)] = name
	}
}

// The mono template icon is the only status cue in mono mode, so it must follow
// the running state too.
func TestMonoIconFollowsRunning(t *testing.T) {
	if !bytes.Equal(pickMonoIcon(true), iconMonoPNG) {
		t.Error("running should show the plain mono icon")
	}
	if !bytes.Equal(pickMonoIcon(false), iconMonoOffPNG) {
		t.Error("stopped should show the slashed mono icon")
	}
}

// A stopped stack must follow the panel too, now that the stopped icon is a
// tinted slash rather than a red L that read everywhere.
func TestIconStateStoppedFollowsPanel(t *testing.T) {
	var applied [][]byte
	s := newIconState()
	s.apply = func(icon []byte) { applied = append(applied, icon) }

	s.setLight(true)
	if s.last != iconKindStoppedLight || !bytes.Equal(applied[len(applied)-1], iconOffDarkPNG) {
		t.Error("stopped on a light panel should show the dark slashed icon")
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

// While a start or stop runs the icon blinks between the stopped and running
// icons, whatever lerd's state is, and settles on the real state afterwards.
func TestIconStatePulseAlternatesThenSettles(t *testing.T) {
	var applied [][]byte
	s := newIconState()
	s.apply = func(icon []byte) { applied = append(applied, icon) }

	s.setPulsing(true)
	s.pulseStep()
	if !bytes.Equal(applied[len(applied)-1], iconWhitePNG) {
		t.Fatal("first pulse step should light the running icon")
	}
	s.pulseStep()
	if !bytes.Equal(applied[len(applied)-1], iconOffWhitePNG) {
		t.Fatal("second pulse step should go back to the stopped icon")
	}
	s.pulseStep()
	s.setRunning(false)
	s.setPulsing(false)
	if s.last != iconKindStoppedDark || !bytes.Equal(applied[len(applied)-1], iconOffWhitePNG) {
		t.Error("ending the pulse should settle on the real stopped state")
	}
}

// With lerd-ui down the snapshot is empty, but the high-contrast choice is saved
// locally and must still hold, or the stopped icon can vanish on a mixed theme.
func TestHighContrastSurvivesUnreachableUI(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetHighContrastTrayIcon(true)
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	var applied [][]byte
	s := newIconState()
	s.apply = func(icon []byte) { applied = append(applied, icon) }

	applyIcon(s, &Snapshot{})

	if len(applied) == 0 || !bytes.Equal(applied[len(applied)-1], iconOffRedPNG) {
		t.Error("a stopped lerd with high contrast saved should show the red slashed icon")
	}
}
