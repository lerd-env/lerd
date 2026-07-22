//go:build linux

package power

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/godbus/dbus/v5"
)

// supply is one entry in a fake /sys/class/power_supply.
type supply struct {
	name   string
	kind   string
	online string
}

// fakeSysfs points the sysfs readers at a fixture tree. profile is written only
// when non-empty, so the common case of hardware without an ACPI platform
// profile is covered too.
func fakeSysfs(t *testing.T, profile string, supplies ...supply) {
	t.Helper()
	root := t.TempDir()

	supplyRoot := filepath.Join(root, "power_supply")
	if err := os.MkdirAll(supplyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, s := range supplies {
		dir := filepath.Join(supplyRoot, s.name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "type"), []byte(s.kind+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if s.online != "" {
			if err := os.WriteFile(filepath.Join(dir, "online"), []byte(s.online+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	profilePath := filepath.Join(root, "platform_profile")
	if profile != "" {
		if err := os.WriteFile(profilePath, []byte(profile+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	prevSupply, prevProfile := powerSupplyRoot, platformProfile
	powerSupplyRoot, platformProfile = supplyRoot, profilePath
	t.Cleanup(func() { powerSupplyRoot, platformProfile = prevSupply, prevProfile })
}

// noBus makes every property read fail, which is what a container, a headless
// box, or an install without UPower looks like.
func noBus(t *testing.T) {
	t.Helper()
	stubBus(t, func(dest, path, iface, prop string) (dbus.Variant, error) {
		return dbus.Variant{}, errors.New("no such service")
	})
}

func stubBus(t *testing.T, fn func(dest, path, iface, prop string) (dbus.Variant, error)) {
	t.Helper()
	prev := systemBusProperty
	systemBusProperty = fn
	t.Cleanup(func() { systemBusProperty = prev })
}

// bus answers OnBattery and ActiveProfile for the named service only, so the
// tests can model a host running UPower without power-profiles-daemon and the
// two spellings of the profiles service independently.
func bus(t *testing.T, onBattery bool, profileDest, profile string) {
	t.Helper()
	stubBus(t, func(dest, path, iface, prop string) (dbus.Variant, error) {
		switch {
		case dest == upowerDest && prop == "OnBattery":
			return dbus.MakeVariant(onBattery), nil
		case dest == profileDest && prop == "ActiveProfile":
			return dbus.MakeVariant(profile), nil
		}
		return dbus.Variant{}, errors.New("no such service")
	})
}

const (
	ppdModern = "org.freedesktop.UPower.PowerProfiles"
	ppdLegacy = "net.hadess.PowerProfiles"
)

func TestDetectStateFromSysfs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		profile  string
		supplies []supply
		want     State
	}{
		{"desktop with no supplies at all", "", nil, Mains},
		{"laptop plugged in", "", []supply{{"AC", "Mains", "1"}, {"BAT0", "Battery", ""}}, Mains},
		{"laptop unplugged", "", []supply{{"AC", "Mains", "0"}, {"BAT0", "Battery", ""}}, Battery},
		// A wireless mouse or keyboard registers as a Battery supply on a
		// mains-powered desktop; treating that as "on battery" would loosen
		// the cadence on a machine that is plugged into the wall.
		{"desktop with a peripheral battery", "", []supply{{"hidpp_battery_0", "Battery", "1"}}, Mains},
		{"power-saver platform profile", "low-power", []supply{{"AC", "Mains", "1"}}, LowPower},
		{"balanced platform profile", "balanced", []supply{{"AC", "Mains", "1"}}, Mains},
		// The profile can be selected while plugged in, and when it is the
		// user has asked for less background work regardless of the source.
		{"platform profile outranks the source", "low-power", []supply{{"AC", "Mains", "0"}}, LowPower},
	} {
		t.Run(tc.name, func(t *testing.T) {
			noBus(t)
			fakeSysfs(t, tc.profile, tc.supplies...)
			if got := detectState(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDetectStateFromBus(t *testing.T) {
	for _, tc := range []struct {
		name        string
		onBattery   bool
		profileDest string
		profile     string
		want        State
	}{
		{"plugged in, balanced", false, ppdModern, "balanced", Mains},
		{"on battery, balanced", true, ppdModern, "balanced", Battery},
		{"power-saver on the current service name", false, ppdModern, "power-saver", LowPower},
		{"power-saver on the legacy service name", false, ppdLegacy, "power-saver", LowPower},
		{"performance profile is not low power", true, ppdModern, "performance", Battery},
		// No power-profiles-daemon on the bus at all: the source still decides.
		{"upower only", true, "", "", Battery},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus(t, tc.onBattery, tc.profileDest, tc.profile)
			fakeSysfs(t, "", supply{"AC", "Mains", "1"})
			if got := detectState(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// UPower knows the real adapters; sysfs is only the fallback for hosts that
// don't run it, so a bus answer must not be second-guessed by the fixture tree.
func TestBusOutranksSysfs(t *testing.T) {
	bus(t, false, ppdModern, "balanced")
	fakeSysfs(t, "", supply{"AC", "Mains", "0"})

	if got := detectState(); got != Mains {
		t.Errorf("bus says plugged in, sysfs says unplugged: got %v, want mains", got)
	}
}

// A daemon that answers with the wrong type must not be read as an answer, or
// a malformed reply would mask the sysfs fallback.
func TestNonBoolOnBatteryFallsBackToSysfs(t *testing.T) {
	stubBus(t, func(dest, path, iface, prop string) (dbus.Variant, error) {
		return dbus.MakeVariant("yes"), nil
	})
	fakeSysfs(t, "", supply{"AC", "Mains", "0"})

	if got := detectState(); got != Battery {
		t.Errorf("got %v, want battery from the sysfs fallback", got)
	}
}
