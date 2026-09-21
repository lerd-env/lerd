package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// MacosThemeID is the id the macOS desktop theme takes in the picker. It is
// reserved: a user theme file of the same name would be shadowed by it.
const MacosThemeID = "macos"

// globalPreferences is the domain macOS records the accent in, as the file
// cfprefsd writes it to.
const globalPreferences = ".GlobalPreferences.plist"

// macosMulticolor is the accent an account that never opened the picker is on.
// macOS records no key for it and paints its controls the system blue.
const macosMulticolor = "4"

// macosAccents is the name and the hex macOS paints each accent with, keyed by
// the index it records. macOS publishes the index, not the colour, and an index
// that is not in here is left alone rather than painted with a neighbour's tone.
var macosAccents = map[string]struct{ name, hex string }{
	"-1": {"graphite", "#8c8c8c"},
	"0":  {"red", "#ff5257"},
	"1":  {"orange", "#f7821b"},
	"2":  {"yellow", "#ffc600"},
	"3":  {"green", "#62ba46"},
	"4":  {"blue", "#007aff"},
	"5":  {"purple", "#a550a7"},
	"6":  {"pink", "#f74f9e"},
}

// macosDesktop reads macOS as a desktop to follow. An accent change lands in the
// global preferences domain, which cfprefsd rewrites whole and stages beside the
// old one, so the watch goes on the directory and filters for the name.
func macosDesktop() Desktop {
	theme := macosTheme(macosAccentIndex())
	if theme == nil {
		return Desktop{}
	}
	return Desktop{
		Theme:      theme,
		WatchDir:   macosPrefsDir(),
		WatchNames: []string{globalPreferences},
	}
}

func macosPrefsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Preferences")
}

// macosTheme turns the accent macOS is on into a dashboard theme. Apple's
// surfaces are not the user's choice and the built-in macOS theme already offers
// them, so the entry lends its accent and leaves lerd's surfaces alone.
func macosTheme(index string) *UITheme {
	accent, ok := macosAccents[index]
	if !ok {
		return nil
	}
	return &UITheme{
		ID:   MacosThemeID,
		Name: "macOS (" + accent.name + ")",
		// The desktop has one accent, not one per mode, so it fills both slots.
		Accent:     accent.hex,
		AccentDark: accent.hex,
		Source:     UIThemeSourceDesktop,
	}
}

// macosAccentIndex returns the accent macOS is on. An account that never opened
// the accent picker records no key at all, and that is multicolor; a machine
// where defaults cannot be run is no macOS to follow. The key is missing in both
// cases, so whether the reader is there at all is what tells them apart.
func macosAccentIndex() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	if _, err := exec.LookPath("defaults"); err != nil {
		return ""
	}
	if v := desktopToolOutput("defaults", "read", "-g", "AppleAccentColor"); v != "" {
		return v
	}
	return macosMulticolor
}
