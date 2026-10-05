package config

import "os/exec"

// macosAccentIndex returns the accent macOS is on. An account that never opened
// the accent picker records no key at all, and that is multicolor; a machine
// where defaults cannot be run is no macOS to follow. The key is missing in both
// cases, so whether the reader is there at all is what tells them apart.
func macosAccentIndex() string {
	if _, err := exec.LookPath("defaults"); err != nil {
		return ""
	}
	if v := desktopToolOutput("defaults", "read", "-g", "AppleAccentColor"); v != "" {
		return v
	}
	return macosMulticolor
}
