package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// stubDesktopTool puts a fake gsettings or dconf on PATH printing what the real
// one prints for the accent key, quotes included. PATH is replaced rather than
// prepended so a machine that has the real tool cannot answer the test.
func stubDesktopTool(t *testing.T, tool, output string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf("#!/bin/sh\necho %q\n", output)
	if err := os.WriteFile(filepath.Join(dir, tool), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return dir
}

// stubDesktopTools stubs both readers into one directory, since the fallback path
// needs them on the same PATH.
func stubDesktopTools(t *testing.T, dconf, gsettings string) {
	t.Helper()
	dir := stubDesktopTool(t, "dconf", dconf)
	script := fmt.Sprintf("#!/bin/sh\necho %q\n", gsettings)
	if err := os.WriteFile(filepath.Join(dir, "gsettings"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestGnomeThemeMapsTheAccentsGnomeOffers(t *testing.T) {
	want := map[string]string{
		"blue": "#3584e4", "teal": "#2190a4", "green": "#3a944a",
		"yellow": "#c88800", "orange": "#ed5b00", "red": "#e62d42",
		"pink": "#d56199", "purple": "#9141ac", "slate": "#6f8396",
		"brown": "#b39169",
	}
	for name, hex := range want {
		theme := gnomeTheme(name, "Adwaita")
		if theme == nil {
			t.Errorf("gnomeTheme(%q) = nil, want %s", name, hex)
			continue
		}
		if theme.Accent != hex || theme.AccentDark != hex {
			t.Errorf("gnomeTheme(%q) accent = %q/%q, want %s", name, theme.Accent, theme.AccentDark, hex)
		}
	}
}

// Ubuntu's libadwaita paints each accent with its Yaru tone instead, so the
// dashboard follows what the desktop actually shows, not the name alone.
func TestGnomeThemeWearsYaruTonesUnderAYaruTheme(t *testing.T) {
	want := map[string]string{
		"blue": "#0073e5", "teal": "#308280", "green": "#4b8501",
		"yellow": "#c88800", "orange": "#e95420", "red": "#da3450",
		"pink": "#b34cb3", "purple": "#7764d8", "slate": "#657b69",
		"brown": "#b39169",
	}
	for name, hex := range want {
		theme := gnomeTheme(name, "Yaru-olive-dark")
		if theme == nil || theme.Accent != hex || theme.AccentDark != hex {
			t.Errorf("gnomeTheme(%q, Yaru) = %+v, want %s", name, theme, hex)
		}
	}
}

// A GNOME that grows a tenth accent must not be painted with a ninth one's
// colour: an accent we do not know is no accent.
func TestGnomeThemeRefusesAnAccentItDoesNotKnow(t *testing.T) {
	if theme := gnomeTheme("chartreuse", "Adwaita"); theme != nil {
		t.Errorf("gnomeTheme(\"chartreuse\") = %+v, want nil", theme)
	}
}

// GNOME's surfaces are not the user's choice, and the built-in Adwaita theme
// already offers them, so the desktop entry lends its accent alone.
func TestGnomeThemeLendsOnlyItsAccent(t *testing.T) {
	theme := gnomeTheme("purple", "Adwaita")
	if theme == nil {
		t.Fatal("gnomeTheme(\"purple\") = nil")
	}
	if theme.ID != GnomeThemeID {
		t.Errorf("ID = %q, want %q", theme.ID, GnomeThemeID)
	}
	if theme.Name != "GNOME (purple)" {
		t.Errorf("Name = %q, want the accent named in it", theme.Name)
	}
	if theme.Source != UIThemeSourceDesktop {
		t.Errorf("Source = %q, want %q", theme.Source, UIThemeSourceDesktop)
	}
	if theme.Bg != "" || theme.Card != "" || theme.Border != "" || theme.Muted != "" {
		t.Errorf("GNOME lent surfaces: bg %q card %q border %q muted %q", theme.Bg, theme.Card, theme.Border, theme.Muted)
	}
}

func TestGnomeAccentNameReadsWhatTheUserRecorded(t *testing.T) {
	stubDesktopTools(t, "'purple'", "'blue'")
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	if got := gnomeInterface("accent-color"); got != "purple" {
		t.Errorf("gnomeInterface(accent-color) = %q, want purple", got)
	}
}

// Most GNOME users never touch the accent picker, so a session that says GNOME
// gets the schema default instead.
func TestGnomeAccentNameFallsBackToTheSessionDefault(t *testing.T) {
	stubDesktopTools(t, "", "'teal'")
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
	if got := gnomeInterface("accent-color"); got != "teal" {
		t.Errorf("gnomeInterface(accent-color) = %q, want teal", got)
	}
}

// The GNOME schemas are installed by half the desktop packages out there, so
// what gsettings answers off a GNOME session proves nothing.
func TestGnomeAccentNameStaysQuietOnAnotherDesktop(t *testing.T) {
	stubDesktopTools(t, "", "'blue'")
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	if got := gnomeInterface("accent-color"); got != "" {
		t.Errorf("gnomeInterface(accent-color) = %q, want nothing off a GNOME session", got)
	}
}

func TestGnomeDesktopWatchesTheDconfDatabase(t *testing.T) {
	stubDesktopTools(t, "'green'", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	d := gnomeDesktop()
	if d.Theme == nil {
		t.Fatal("gnomeDesktop() lent no theme")
	}
	if want := filepath.Join(xdgConfigHome(), "dconf"); d.WatchDir != want {
		t.Errorf("WatchDir = %q, want %q", d.WatchDir, want)
	}
	if len(d.WatchNames) != 1 || d.WatchNames[0] != "user" {
		t.Errorf("WatchNames = %v, want just the user database", d.WatchNames)
	}
}
