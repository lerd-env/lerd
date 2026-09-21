package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PlasmaThemeID is the id the Plasma desktop theme takes in the picker. It is
// reserved: a user theme file of the same name would be shadowed by it.
const PlasmaThemeID = "plasma"

// kdeGlobals is the file Plasma writes its accent into, and the one it copies
// the active colour scheme into, so both arrive from a single read.
const kdeGlobals = "kdeglobals"

// plasmaDesktop reads Plasma as a desktop to follow. KConfig rewrites the file
// whole and stages it beside the old one, so the watch goes on the directory and
// filters for the name rather than sitting on a path that gets replaced.
func plasmaDesktop() Desktop {
	theme := plasmaTheme()
	if theme == nil {
		return Desktop{}
	}
	return Desktop{Theme: theme, WatchDir: xdgConfigHome(), WatchNames: []string{kdeGlobals}}
}

// plasmaTheme reads the active Plasma colours as a dashboard theme, or returns
// nil where there is no Plasma to read.
func plasmaTheme() *UITheme {
	groups := parseKdeConfig(filepath.Join(xdgConfigHome(), kdeGlobals))
	accent := kdeColor(groups["General"]["AccentColor"])
	// An accent in kdeglobals is one the user picked in Plasma's own settings, so
	// it stands on its own. The scheme's selection colour does not: kdeglobals is
	// written by any machine that has ever run a Qt application, and only a
	// session that says Plasma makes that colour the accent in front of the user.
	if accent == "" && inDesktopSession("KDE") {
		accent = kdeColor(groups["Colors:Selection"]["BackgroundNormal"])
	}
	if accent == "" {
		return nil
	}
	theme := &UITheme{
		ID:   PlasmaThemeID,
		Name: plasmaThemeName(groups["General"]["ColorScheme"]),
		// The desktop has one accent, not one per mode, so it fills both slots.
		Accent:     accent,
		AccentDark: accent,
		Source:     UIThemeSourceDesktop,
	}
	// A dashboard palette's surfaces are its dark ones, so a light scheme lends
	// its accent and nothing else. Whether a scheme is dark is decided off the
	// window background rather than the scheme's name, which can be anything.
	if kdeIsDark(groups["Colors:Window"]["BackgroundNormal"]) {
		// The view background is the darkest surface a scheme has, the window
		// background is the one a card sits on, and the alternate window
		// background is the step between them a border wants.
		theme.Bg = kdeColor(groups["Colors:View"]["BackgroundNormal"])
		theme.Card = kdeColor(groups["Colors:Window"]["BackgroundNormal"])
		theme.Border = kdeColor(groups["Colors:Window"]["BackgroundAlternate"])
	} else {
		// A light scheme has one tone the dashboard can use: Plasma tints the
		// chrome around its content with the window background and leaves the
		// content itself on the view background, which light mode already paints
		// white.
		theme.ChromeLight = kdeColor(groups["Colors:Window"]["BackgroundNormal"])
	}
	return theme
}

// plasmaThemeName labels the entry with the scheme it is following. Plasma stops
// writing ColorScheme once a custom accent turns the scheme into a generated
// one, and the entry still has to say what it is.
func plasmaThemeName(scheme string) string {
	if scheme = strings.TrimSpace(scheme); scheme == "" {
		return "Plasma"
	}
	return "Plasma (" + scheme + ")"
}

// parseKdeConfig reads a KConfig file into group to key to value. Group names
// are matched whole, so the nested headers KDE writes, like
// [Colors:Header][Inactive], simply never match a lookup.
func parseKdeConfig(path string) map[string]map[string]string {
	groups := map[string]map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return groups
	}
	defer func() { _ = f.Close() }()
	group := ""
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			group = line[1 : len(line)-1]
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || group == "" {
			continue
		}
		if groups[group] == nil {
			groups[group] = map[string]string{}
		}
		groups[group][strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return groups
}

// kdeColor turns the "r,g,b" triple KConfig stores colours as into a hex colour,
// or an empty string for anything that is not one.
func kdeColor(v string) string {
	r, g, b, ok := kdeRGB(v)
	if !ok {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// kdeIsDark says whether a surface is a dark one, weighting the channels the way
// perceived brightness does.
func kdeIsDark(v string) bool {
	r, g, b, ok := kdeRGB(v)
	if !ok {
		return false
	}
	return 0.299*float64(r)+0.587*float64(g)+0.114*float64(b) < 128
}

func kdeRGB(v string) (r, g, b int, ok bool) {
	parts := strings.Split(strings.TrimSpace(v), ",")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 || n > 255 {
			return 0, 0, 0, false
		}
		out[i] = n
	}
	return out[0], out[1], out[2], true
}
