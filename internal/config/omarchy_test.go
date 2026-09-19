package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeOmarchyTheme lays out the state directory Omarchy keeps its active theme
// in, so a test can hand the reader a real tree rather than a stub.
func writeOmarchyTheme(t *testing.T, name, colors string) {
	t.Helper()
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	dir := filepath.Join(state, "omarchy", "current", "theme")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if colors != "" {
		if err := os.WriteFile(filepath.Join(dir, "colors.toml"), []byte(colors), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if name != "" {
		parent := filepath.Join(state, "omarchy", "current", "theme.name")
		if err := os.WriteFile(parent, []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOmarchyThemeReadsTheActiveTheme(t *testing.T) {
	writeOmarchyTheme(t, "tokyo-night", `
mode = "dark"
accent = "#7aa2f7"
muted = "#565f89"
background = "#1a1b26"
lighter_background = "#24283b"
selection = "#283457"
`)
	theme := OmarchyTheme()
	if theme == nil {
		t.Fatal("OmarchyTheme() = nil, want the active theme")
	}
	if theme.ID != OmarchyThemeID {
		t.Errorf("ID = %q, want %q", theme.ID, OmarchyThemeID)
	}
	if theme.Name != "Omarchy (tokyo-night)" {
		t.Errorf("Name = %q, want the desktop theme named in it", theme.Name)
	}
	if theme.Accent != "#7aa2f7" {
		t.Errorf("Accent = %q, want #7aa2f7", theme.Accent)
	}
	if theme.Bg != "#1a1b26" {
		t.Errorf("Bg = %q, want the background", theme.Bg)
	}
	if theme.Card != "#24283b" {
		t.Errorf("Card = %q, want the lighter background", theme.Card)
	}
	if theme.Border != "#283457" {
		t.Errorf("Border = %q, want the selection colour", theme.Border)
	}
	if theme.Muted != "#565f89" {
		t.Errorf("Muted = %q, want the muted colour", theme.Muted)
	}
}

// A dashboard palette's surfaces are its dark ones, so a light desktop theme
// lends its accent and leaves the surfaces alone rather than putting a pale
// background behind type coloured to sit on a dark card.
func TestOmarchyThemeTakesOnlyTheAccentFromALightTheme(t *testing.T) {
	writeOmarchyTheme(t, "catppuccin-latte", `
mode = "light"
accent = "#1e66f5"
muted = "#8c8fa1"
background = "#eff1f5"
lighter_background = "#e6e9ef"
selection = "#dce0e8"
`)
	theme := OmarchyTheme()
	if theme == nil {
		t.Fatal("OmarchyTheme() = nil, want the active theme")
	}
	if theme.Accent != "#1e66f5" {
		t.Errorf("Accent = %q, want the desktop accent", theme.Accent)
	}
	for label, got := range map[string]string{
		"Bg": theme.Bg, "Card": theme.Card, "Border": theme.Border, "Muted": theme.Muted,
	} {
		if got != "" {
			t.Errorf("%s = %q, want it left to the dashboard's own dark surfaces", label, got)
		}
	}
}

// The dark accent is filled from the same colour rather than derived, because
// the desktop chose one accent and it already reads against its own surfaces.
func TestOmarchyThemeFillsBothAccentSlots(t *testing.T) {
	writeOmarchyTheme(t, "nord", "mode = \"dark\"\naccent = \"#81a1c1\"\n")
	theme := OmarchyTheme()
	if theme == nil {
		t.Fatal("OmarchyTheme() = nil, want the active theme")
	}
	if theme.Accent != "#81a1c1" || theme.AccentDark != "#81a1c1" {
		t.Errorf("accents = %q/%q, want both the desktop accent", theme.Accent, theme.AccentDark)
	}
}

func TestOmarchyThemeAbsentWithoutOmarchy(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if theme := OmarchyTheme(); theme != nil {
		t.Errorf("OmarchyTheme() = %+v, want nil where Omarchy is not installed", theme)
	}
}

// A theme directory with no colors.toml is an Omarchy too old to read rather
// than an error to report: there is nothing the user could fix in it.
func TestOmarchyThemeAbsentWithoutColors(t *testing.T) {
	writeOmarchyTheme(t, "vantablack", "")
	if theme := OmarchyTheme(); theme != nil {
		t.Errorf("OmarchyTheme() = %+v, want nil when the theme ships no colors.toml", theme)
	}
}

// An accent that is not a colour makes the whole entry unusable, and the picker
// showing a broken swatch is worse than showing nothing.
func TestOmarchyThemeAbsentWhenAccentUnusable(t *testing.T) {
	writeOmarchyTheme(t, "broken", "mode = \"dark\"\naccent = \"not a colour\"\n")
	if theme := OmarchyTheme(); theme != nil {
		t.Errorf("OmarchyTheme() = %+v, want nil when the accent will not parse", theme)
	}
}

// The name file is what omarchy-theme-set writes beside the theme, but a theme
// that arrived some other way still has a directory, so fall back to it.
func TestOmarchyThemeNamesItselfWithoutTheNameFile(t *testing.T) {
	writeOmarchyTheme(t, "", "accent = \"#8d8d8d\"\n")
	theme := OmarchyTheme()
	if theme == nil {
		t.Fatal("OmarchyTheme() = nil, want the theme without its name file")
	}
	if theme.Name != "Omarchy" {
		t.Errorf("Name = %q, want the bare product name", theme.Name)
	}
}
