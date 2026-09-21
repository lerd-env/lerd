package config

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestMacosThemeMapsTheAccentsMacosOffers(t *testing.T) {
	want := map[string]string{
		"-1": "#8c8c8c", "0": "#ff5257", "1": "#f7821b", "2": "#ffc600",
		"3": "#62ba46", "4": "#007aff", "5": "#a550a7", "6": "#f74f9e",
	}
	for index, hex := range want {
		theme := macosTheme(index)
		if theme == nil {
			t.Errorf("macosTheme(%q) = nil, want %s", index, hex)
			continue
		}
		if theme.Accent != hex || theme.AccentDark != hex {
			t.Errorf("macosTheme(%q) accent = %q/%q, want %s", index, theme.Accent, theme.AccentDark, hex)
		}
	}
}

// A macOS that grows a ninth accent must not be painted with an eighth one's
// colour: an accent we do not know is no accent.
func TestMacosThemeRefusesAnAccentItDoesNotKnow(t *testing.T) {
	if theme := macosTheme("7"); theme != nil {
		t.Errorf("macosTheme(\"7\") = %+v, want nil", theme)
	}
}

// Apple's surfaces are not the user's choice, and the built-in macOS theme
// already offers them, so the desktop entry lends its accent alone.
func TestMacosThemeLendsOnlyItsAccent(t *testing.T) {
	theme := macosTheme("5")
	if theme == nil {
		t.Fatal("macosTheme(\"5\") = nil")
	}
	if theme.ID != MacosThemeID {
		t.Errorf("ID = %q, want %q", theme.ID, MacosThemeID)
	}
	if theme.Name != "macOS (purple)" {
		t.Errorf("Name = %q, want the accent named in it", theme.Name)
	}
	if theme.Source != UIThemeSourceDesktop {
		t.Errorf("Source = %q, want %q", theme.Source, UIThemeSourceDesktop)
	}
	if theme.Bg != "" || theme.Card != "" || theme.Border != "" || theme.Muted != "" {
		t.Errorf("macOS lent surfaces: bg %q card %q border %q muted %q", theme.Bg, theme.Card, theme.Border, theme.Muted)
	}
}

func TestMacosAccentIndexReadsWhatTheUserPicked(t *testing.T) {
	requireDarwin(t)
	stubDesktopTool(t, "defaults", "6")
	if got := macosAccentIndex(); got != "6" {
		t.Errorf("macosAccentIndex() = %q, want 6", got)
	}
}

// Most Macs never have the accent picker touched, and that default is
// multicolor, which paints the controls the system blue.
func TestMacosAccentIndexFallsBackToMulticolor(t *testing.T) {
	requireDarwin(t)
	stubDesktopTool(t, "defaults", "")
	if got := macosAccentIndex(); got != macosMulticolor {
		t.Errorf("macosAccentIndex() = %q, want %q", got, macosMulticolor)
	}
}

func TestMacosAccentIndexStaysQuietWithNoReader(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if got := macosAccentIndex(); got != "" {
		t.Errorf("macosAccentIndex() = %q, want nothing off a Mac", got)
	}
}

func TestMacosDesktopWatchesTheGlobalPreferences(t *testing.T) {
	requireDarwin(t)
	stubDesktopTool(t, "defaults", "3")
	d := macosDesktop()
	if d.Theme == nil {
		t.Fatal("macosDesktop() lent no theme")
	}
	if want := filepath.Join(macosPrefsDir(), globalPreferences); filepath.Join(d.WatchDir, d.WatchNames[0]) != want {
		t.Errorf("watching %v, want %q", d, want)
	}
}

func requireDarwin(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("the accent reader only runs on macOS")
	}
}
