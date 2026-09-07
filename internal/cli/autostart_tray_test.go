package cli

import (
	"slices"
	"testing"
)

func TestUnitsToArm_KeepsTrayWhenItIsOn(t *testing.T) {
	withTempXDG(t)

	got := unitsToArm([]string{"lerd-tray.service", "lerd-ui.service"})
	if !slices.Contains(got, "lerd-tray.service") {
		t.Error("enabling autostart with the tray on must arm lerd-tray at login")
	}
}

func TestUnitsToArm_DropsTrayWhenItIsOff(t *testing.T) {
	withTempXDG(t)

	if _, err := setTrayPreference(false); err != nil {
		t.Fatal(err)
	}
	got := unitsToArm([]string{"lerd-tray.service", "lerd-ui.service", "lerd-watcher.service"})
	if slices.Contains(got, "lerd-tray.service") {
		t.Error("enabling autostart must not bring back a tray the user turned off")
	}
	for _, want := range []string{"lerd-ui.service", "lerd-watcher.service"} {
		if !slices.Contains(got, want) {
			t.Errorf("%s must still be armed at login", want)
		}
	}
}
