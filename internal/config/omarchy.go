package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// OmarchyThemeID is the id the desktop theme takes in the picker. It is
// reserved: a user theme file of the same name would be shadowed by it.
const OmarchyThemeID = "omarchy"

// omarchyColors is the part of an Omarchy theme's colors.toml a dashboard
// theme has a use for. Every theme Omarchy ships carries this file, so it is
// read directly rather than inferred from a terminal or editor config.
type omarchyColors struct {
	Mode              string `toml:"mode"`
	Accent            string `toml:"accent"`
	Muted             string `toml:"muted"`
	Selection         string `toml:"selection"`
	Background        string `toml:"background"`
	LighterBackground string `toml:"lighter_background"`
}

// omarchyDesktop reads Omarchy as a desktop to follow. The watch sits on the
// state directory rather than on the theme itself because omarchy-theme-set
// stages the new theme beside the old one and moves it into place, so the path
// being watched would be the one that goes away.
func omarchyDesktop() Desktop {
	theme := OmarchyTheme()
	if theme == nil {
		return Desktop{}
	}
	return Desktop{
		Theme:      theme,
		WatchDir:   OmarchyCurrentDir(),
		WatchNames: []string{"theme", "theme.name"},
	}
}

// OmarchyCurrentDir returns the state directory Omarchy keeps the active theme
// in. The theme under it is a directory that omarchy-theme-set stages beside
// the old one and moves into place, which is why a watch belongs here rather
// than on the theme path itself.
func OmarchyCurrentDir() string {
	return filepath.Join(xdgStateHome(), "omarchy", "current")
}

// OmarchyTheme reads the active Omarchy theme as a dashboard theme, or returns
// nil where there is no Omarchy to read. Anything unreadable is absence rather
// than an error: unlike a hand-written theme file, there is nothing here the
// user could correct.
func OmarchyTheme() *UITheme {
	data, err := os.ReadFile(filepath.Join(OmarchyCurrentDir(), "theme", "colors.toml"))
	if err != nil {
		return nil
	}
	var c omarchyColors
	if err := toml.Unmarshal(data, &c); err != nil {
		return nil
	}
	accent := NormalizeBrandColor(c.Accent)
	if accent == "" {
		return nil
	}
	theme := &UITheme{
		ID:   OmarchyThemeID,
		Name: omarchyThemeName(),
		// The desktop has one accent, not one per mode, so it fills both slots
		// rather than letting the dark one be derived off a tone that was
		// already chosen to read against this theme's own surfaces.
		Accent:     accent,
		AccentDark: accent,
		Source:     UIThemeSourceDesktop,
	}
	// A dashboard palette's surfaces are its dark ones; there is no light set to
	// put a light desktop's background into, and dropping one in would paint a
	// pale card behind type coloured to sit on a dark one. A light theme lends
	// its accent and nothing else.
	if !omarchyIsLight(c.Mode) {
		theme.Bg = NormalizeBrandColor(c.Background)
		theme.Card = NormalizeBrandColor(c.LighterBackground)
		theme.Border = NormalizeBrandColor(c.Selection)
		theme.Muted = NormalizeBrandColor(c.Muted)
	}
	return theme
}

func omarchyIsLight(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), "light")
}

// omarchyThemeName labels the entry with the desktop theme it is following, so
// the picker says which one rather than just naming the desktop.
func omarchyThemeName() string {
	data, err := os.ReadFile(filepath.Join(OmarchyCurrentDir(), "theme.name"))
	name := strings.TrimSpace(string(data))
	if err != nil || name == "" {
		return "Omarchy"
	}
	return "Omarchy (" + name + ")"
}
