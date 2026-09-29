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
	DarkBackground    string `toml:"dark_background"`
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
	// A dark desktop theme lends the dark surfaces and a light one the light
	// surfaces. A light theme's dark_background is the page under its cards, the
	// way lerd's light mode puts white cards on a grey page.
	if omarchyIsLight(c.Mode) {
		theme.BgLight = NormalizeBrandColor(c.DarkBackground)
		if theme.BgLight == "" {
			theme.BgLight = NormalizeBrandColor(c.Background)
		}
		theme.CardLight = NormalizeBrandColor(c.Background)
		theme.BorderLight = NormalizeBrandColor(c.Selection)
	} else {
		theme.Bg = NormalizeBrandColor(c.Background)
		theme.Card = NormalizeBrandColor(c.LighterBackground)
		theme.Border = NormalizeBrandColor(c.Selection)
		theme.Muted = NormalizeBrandColor(c.Muted)
	}
	// A lerd.yaml the theme carries, whether shipped with it or rendered from an
	// Omarchy template, has the last word on any tone it sets.
	if override, _ := omarchyOverride(); override != nil {
		// The desktop's accent fills both modes, so an override's accent must
		// too, or dark mode keeps the one it replaced.
		if override.AccentDark == "" {
			override.AccentDark = override.Accent
		}
		dst := theme.colours()
		for key, v := range override.colours() {
			if *v != "" {
				*dst[key] = *v
			}
		}
	}
	return theme
}

// omarchyOverride reads the optional lerd.yaml in the active Omarchy theme. A
// missing file is (nil, nil).
func omarchyOverride() (*UITheme, error) {
	data, err := os.ReadFile(OmarchyOverridePath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeUITheme(data)
}

// OmarchyOverridePath is where an Omarchy theme, or a lerd.yaml.tpl template
// Omarchy renders into it, keeps the colours lerd should use.
func OmarchyOverridePath() string {
	return filepath.Join(OmarchyCurrentDir(), "theme", "lerd.yaml")
}

// OmarchyCSSPath is the stylesheet an Omarchy theme, or a lerd.css.tpl template,
// adds to the dashboard while it follows that theme.
func OmarchyCSSPath() string {
	return filepath.Join(OmarchyCurrentDir(), "theme", "lerd.css")
}

// OmarchyOverrideError reports a lerd.yaml in the active Omarchy theme that
// could not be used, so the picker can say why rather than silently ignore it.
func OmarchyOverrideError() *UIThemeError {
	if _, err := omarchyOverride(); err != nil {
		return &UIThemeError{File: OmarchyOverridePath(), Error: err.Error()}
	}
	return nil
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

// AdoptOmarchyTheme puts an install that never chose a theme on Omarchy's own,
// reporting whether it did. A choice already made, the default included, stays.
func (c *GlobalConfig) AdoptOmarchyTheme() bool {
	if c.UI.Theme != "" || OmarchyTheme() == nil {
		return false
	}
	c.UI.Theme = OmarchyThemeID
	return true
}
