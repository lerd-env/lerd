//go:build linux

package power

import (
	"os"
	"path/filepath"
	"strings"
)

// sysfs roots, vars so tests can point them at a fixture tree.
var (
	powerSupplyRoot = "/sys/class/power_supply"
	platformProfile = "/sys/firmware/acpi/platform_profile"
)

// detectState reads sysfs. The ACPI platform profile is the closest analogue to
// macOS Low Power Mode and, like it, can be selected while plugged in, so it is
// checked first.
func detectState() State {
	if profile, err := os.ReadFile(platformProfile); err == nil {
		if strings.TrimSpace(string(profile)) == "low-power" {
			return LowPower
		}
	}
	if onBatterySysfs() {
		return Battery
	}
	return Mains
}

// onBatterySysfs reports true only when a mains adapter exists and none is
// online. A host with no adapter at all (desktop, VM, container) reports false
// rather than guessing.
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
