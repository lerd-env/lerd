//go:build linux

package power

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// sysfs roots, vars so tests can point them at a fixture tree.
var (
	powerSupplyRoot = "/sys/class/power_supply"
	platformProfile = "/sys/firmware/acpi/platform_profile"
)

// dbusTimeout bounds every bus call so a wedged UPower can never stall a worker
// write.
const dbusTimeout = 2 * time.Second

// upowerDest, upowerPath and upowerIface own the power source. UPower is on
// every desktop Linux install and answers where sysfs cannot: it knows which
// supplies are real adapters rather than a wireless mouse reporting a battery.
const (
	upowerDest  = "org.freedesktop.UPower"
	upowerPath  = "/org/freedesktop/UPower"
	upowerIface = "org.freedesktop.UPower"
)

// powerProfileServices are the buses that own the power-saver toggle the
// desktops expose, newest name first. power-profiles-daemon moved under the
// UPower name at 0.20 and still claims the old one, so both are tried and the
// first that answers wins.
var powerProfileServices = []struct{ dest, path, iface string }{
	{"org.freedesktop.UPower.PowerProfiles", "/org/freedesktop/UPower/PowerProfiles", "org.freedesktop.UPower.PowerProfiles"},
	{"net.hadess.PowerProfiles", "/net/hadess/PowerProfiles", "net.hadess.PowerProfiles"},
}

// systemBusProperty reads one property off the system bus. A var so tests can
// answer for services this machine may not run.
var systemBusProperty = func(dest, path, iface, prop string) (dbus.Variant, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return dbus.Variant{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbusTimeout)
	defer cancel()
	var v dbus.Variant
	err = conn.Object(dest, dbus.ObjectPath(path)).
		CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, iface, prop).Store(&v)
	return v, err
}

// detectState prefers the system bus and falls back to sysfs. The power-saver
// profile is the closest analogue to macOS Low Power Mode and, like it, can be
// selected while plugged in, so it is checked first.
func detectState() State {
	if busElseFile(lowPowerDBus, lowPowerSysfs) {
		return LowPower
	}
	if busElseFile(onBatteryDBus, onBatterySysfs) {
		return Battery
	}
	return Mains
}

// busElseFile takes the bus answer when a service replied and reads sysfs
// otherwise, so a host with no UPower (a container, a headless box, a minimal
// install) still gets an answer instead of defaulting blind.
func busElseFile(bus func() (bool, bool), sysfs func() bool) bool {
	if v, answered := bus(); answered {
		return v
	}
	return sysfs()
}

// lowPowerDBus asks power-profiles-daemon which profile is active.
func lowPowerDBus() (low, answered bool) {
	for _, svc := range powerProfileServices {
		v, err := systemBusProperty(svc.dest, svc.path, svc.iface, "ActiveProfile")
		if err != nil {
			continue
		}
		if profile, ok := v.Value().(string); ok {
			return profile == "power-saver", true
		}
	}
	return false, false
}

// onBatteryDBus asks UPower for the power source.
func onBatteryDBus() (battery, answered bool) {
	v, err := systemBusProperty(upowerDest, upowerPath, upowerIface, "OnBattery")
	if err != nil {
		return false, false
	}
	b, ok := v.Value().(bool)
	return b, ok
}

// lowPowerSysfs reads the ACPI platform profile, which only some hardware
// exposes; power-profiles-daemon drives it there and uses the CPU driver
// elsewhere, which is why the bus is asked first.
func lowPowerSysfs() bool {
	profile, err := os.ReadFile(platformProfile)
	return err == nil && strings.TrimSpace(string(profile)) == "low-power"
}

// onBatterySysfs reports true only when a mains adapter exists and none is
// online. A host with no adapter at all (desktop, VM, container) reports false
// rather than guessing, and a peripheral reporting its own battery is ignored.
func onBatterySysfs() bool {
	entries, err := filepath.Glob(filepath.Join(powerSupplyRoot, "*"))
	if err != nil {
		return false
	}
	sawMains := false
	for _, dir := range entries {
		kind, err := os.ReadFile(filepath.Join(dir, "type"))
		if err != nil || strings.TrimSpace(string(kind)) != "Mains" {
			continue
		}
		sawMains = true
		online, err := os.ReadFile(filepath.Join(dir, "online"))
		if err == nil && strings.TrimSpace(string(online)) == "1" {
			return false
		}
	}
	return sawMains
}
