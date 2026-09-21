package config

import (
	"os"
	"path/filepath"
	"testing"
)

// breezeDark is the part of a Plasma colour scheme a dashboard theme has a use
// for, trimmed out of a real kdeglobals rather than invented: Plasma copies the
// active scheme into this file, so these groups are always there.
const breezeDark = `
[Colors:Button]
BackgroundNormal=41,44,48

[Colors:Selection]
BackgroundNormal=61,174,233

[Colors:View]
BackgroundAlternate=29,31,34
BackgroundNormal=20,22,24

[Colors:Window]
BackgroundAlternate=41,44,48
BackgroundNormal=32,35,38
ForegroundInactive=161,169,177

[General]
AccentColor=61,212,37
ColorScheme=Breeze Dark

[KDE]
LookAndFeelPackage=org.kde.breezedark.desktop
`

// writeKdeGlobals lays out the config file Plasma keeps its accent and its
// active colour scheme in, so the reader is handed a real file.
func writeKdeGlobals(t *testing.T, desktop, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_CURRENT_DESKTOP", desktop)
	if body == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "kdeglobals"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPlasmaThemeReadsTheAccentAndTheSchemeSurfaces(t *testing.T) {
	writeKdeGlobals(t, "KDE", breezeDark)
	theme := plasmaTheme()
	if theme == nil {
		t.Fatal("plasmaTheme() = nil, want the active scheme")
	}
	if theme.ID != PlasmaThemeID {
		t.Errorf("ID = %q, want %q", theme.ID, PlasmaThemeID)
	}
	if theme.Name != "Plasma (Breeze Dark)" {
		t.Errorf("Name = %q, want the scheme named in it", theme.Name)
	}
	if theme.Accent != "#3dd425" || theme.AccentDark != "#3dd425" {
		t.Errorf("Accent = %q/%q, want #3dd425 in both slots", theme.Accent, theme.AccentDark)
	}
	if theme.Bg != "#141618" {
		t.Errorf("Bg = %q, want the view background", theme.Bg)
	}
	if theme.Card != "#202326" {
		t.Errorf("Card = %q, want the window background", theme.Card)
	}
	if theme.Border != "#292c30" {
		t.Errorf("Border = %q, want the alternate window background", theme.Border)
	}
	if theme.Source != UIThemeSourceDesktop {
		t.Errorf("Source = %q, want %q", theme.Source, UIThemeSourceDesktop)
	}
}

// A light scheme lends its accent and nothing else, the same way a light Omarchy
// theme does: the surface fields are the dark ones.
func TestPlasmaThemeLendsOnlyItsAccentOnALightScheme(t *testing.T) {
	writeKdeGlobals(t, "KDE", `
[Colors:View]
BackgroundNormal=255,255,255

[Colors:Window]
BackgroundAlternate=227,229,231
BackgroundNormal=239,240,241

[General]
AccentColor=61,174,233
`)
	theme := plasmaTheme()
	if theme == nil {
		t.Fatal("plasmaTheme() = nil, want the accent of a light scheme")
	}
	if theme.Accent != "#3daee9" {
		t.Errorf("Accent = %q, want #3daee9", theme.Accent)
	}
	if theme.Bg != "" || theme.Card != "" || theme.Border != "" {
		t.Errorf("light scheme lent surfaces: bg %q card %q border %q", theme.Bg, theme.Card, theme.Border)
	}
	// The one tone a light scheme does have a use for: Plasma tints the chrome
	// around its content, and the window background is what it tints it with.
	if theme.ChromeLight != "#eff0f1" {
		t.Errorf("ChromeLight = %q, want the window background", theme.ChromeLight)
	}
}

// A dark scheme says nothing about what light mode should look like, so the
// entry inherits the built-in's tint rather than inventing one.
func TestPlasmaThemeLendsNoLightChromeOnADarkScheme(t *testing.T) {
	writeKdeGlobals(t, "KDE", breezeDark)
	theme := plasmaTheme()
	if theme == nil {
		t.Fatal("plasmaTheme() = nil")
	}
	if theme.ChromeLight != "" {
		t.Errorf("ChromeLight = %q, want nothing off a dark scheme", theme.ChromeLight)
	}
}

// Plasma stops writing ColorScheme once a custom accent turns the scheme into a
// generated one, and the entry still has to be labelled.
func TestPlasmaThemeNameFallsBackToTheDesktop(t *testing.T) {
	writeKdeGlobals(t, "KDE", `
[Colors:View]
BackgroundNormal=20,22,24

[Colors:Window]
BackgroundNormal=32,35,38

[General]
AccentColor=61,212,37
ColorSchemeHash=0efb452114ece4d3338057cb89f434aff85d80d0
`)
	theme := plasmaTheme()
	if theme == nil {
		t.Fatal("plasmaTheme() = nil, want the accent")
	}
	if theme.Name != "Plasma" {
		t.Errorf("Name = %q, want %q", theme.Name, "Plasma")
	}
}

// A stock Plasma has no AccentColor until the user picks one, so the scheme's
// selection colour stands in for it.
func TestPlasmaThemeFallsBackToTheSelectionColourInASession(t *testing.T) {
	writeKdeGlobals(t, "KDE", `
[Colors:Selection]
BackgroundNormal=61,174,233

[Colors:View]
BackgroundNormal=20,22,24

[Colors:Window]
BackgroundNormal=32,35,38
`)
	theme := plasmaTheme()
	if theme == nil {
		t.Fatal("plasmaTheme() = nil, want the selection colour as the accent")
	}
	if theme.Accent != "#3daee9" {
		t.Errorf("Accent = %q, want the selection colour #3daee9", theme.Accent)
	}
}

// kdeglobals is written by any machine that has ever run a Qt application, so a
// scheme alone is not evidence that Plasma is what the user is looking at.
func TestPlasmaThemeIgnoresASchemeOnAnotherDesktop(t *testing.T) {
	writeKdeGlobals(t, "Hyprland", `
[Colors:Selection]
BackgroundNormal=61,174,233

[Colors:Window]
BackgroundNormal=32,35,38
`)
	if theme := plasmaTheme(); theme != nil {
		t.Errorf("plasmaTheme() = %+v, want nil off a Plasma session", theme)
	}
}

// An accent the user recorded in Plasma's settings is evidence on its own, which
// is what keeps the entry there when lerd-ui started before the session exported
// its environment.
func TestPlasmaThemeAcceptsARecordedAccentWithoutTheSession(t *testing.T) {
	writeKdeGlobals(t, "", breezeDark)
	if theme := plasmaTheme(); theme == nil {
		t.Error("plasmaTheme() = nil, want the accent the user recorded")
	}
}

func TestPlasmaThemeWithoutTheFile(t *testing.T) {
	writeKdeGlobals(t, "KDE", "")
	if theme := plasmaTheme(); theme != nil {
		t.Errorf("plasmaTheme() = %+v, want nil with no kdeglobals", theme)
	}
}

func TestPlasmaDesktopWatchesTheConfigFile(t *testing.T) {
	writeKdeGlobals(t, "KDE", breezeDark)
	d := plasmaDesktop()
	if d.Theme == nil {
		t.Fatal("plasmaDesktop() lent no theme")
	}
	if d.WatchDir != xdgConfigHome() {
		t.Errorf("WatchDir = %q, want %q", d.WatchDir, xdgConfigHome())
	}
	if len(d.WatchNames) != 1 || d.WatchNames[0] != "kdeglobals" {
		t.Errorf("WatchNames = %v, want just kdeglobals", d.WatchNames)
	}
}
