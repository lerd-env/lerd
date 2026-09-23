package config

import (
	"path/filepath"
	"strings"
)

// GnomeThemeID is the id the GNOME desktop theme takes in the picker. It is
// reserved: a user theme file of the same name would be shadowed by it.
const GnomeThemeID = "gnome"

// gnomeAccents is libadwaita's own hex for each accent GNOME offers. GNOME
// publishes the name, not the colour, and an accent that is not in here is left
// alone rather than painted with a neighbour's tone.
var gnomeAccents = map[string]string{
	"blue":   "#3584e4",
	"teal":   "#2190a4",
	"green":  "#3a944a",
	"yellow": "#c88800",
	"orange": "#ed5b00",
	"red":    "#e62d42",
	"pink":   "#d56199",
	"purple": "#9141ac",
	"slate":  "#6f8396",
	"brown":  "#b39169",
}

// yaruAccents is the tone Ubuntu's patched libadwaita paints each accent with
// under a Yaru theme, read off the default-light-yaru-*.css it ships.
var yaruAccents = map[string]string{
	"blue":   "#0073e5",
	"teal":   "#308280",
	"green":  "#4b8501",
	"yellow": "#c88800",
	"orange": "#e95420",
	"red":    "#da3450",
	"pink":   "#b34cb3",
	"purple": "#7764d8",
	"slate":  "#657b69",
	"brown":  "#b39169",
}

// gnomeDesktop reads GNOME as a desktop to follow. An accent change lands in
// dconf's database, which is one file the whole of dconf is rewritten into.
func gnomeDesktop() Desktop {
	theme := gnomeTheme(gnomeInterface("accent-color"), gnomeInterface("gtk-theme"))
	if theme == nil {
		return Desktop{}
	}
	return Desktop{
		Theme:      theme,
		WatchDir:   filepath.Join(xdgConfigHome(), "dconf"),
		WatchNames: []string{"user"},
	}
}

// gnomeTheme turns the accent GNOME is on into a dashboard theme, in the tone
// the GTK theme paints it with. GNOME's surfaces are not the user's choice and
// the built-in Adwaita theme already offers them, so the entry lends its accent
// and leaves lerd's surfaces alone.
func gnomeTheme(accent, gtkTheme string) *UITheme {
	accents := gnomeAccents
	if strings.HasPrefix(gtkTheme, "Yaru") {
		accents = yaruAccents
	}
	hex := accents[accent]
	if hex == "" {
		return nil
	}
	return &UITheme{
		ID:   GnomeThemeID,
		Name: "GNOME (" + accent + ")",
		// The desktop has one accent, not one per mode, so it fills both slots.
		Accent:     hex,
		AccentDark: hex,
		Source:     UIThemeSourceDesktop,
	}
}

// gnomeInterface returns a key of GNOME's interface settings. A value recorded
// in dconf is one the user picked, so it counts wherever it is found; the
// schema default counts only in a GNOME session, since half the desktop
// packages out there install the schema it would be read from. The two sided
// gate is also what keeps the entry there when lerd-ui started before the
// session exported its environment.
func gnomeInterface(key string) string {
	if v := desktopToolOutput("dconf", "read", "/org/gnome/desktop/interface/"+key); v != "" {
		return v
	}
	if !inDesktopSession("GNOME") {
		return ""
	}
	return desktopToolOutput("gsettings", "get", "org.gnome.desktop.interface", key)
}
