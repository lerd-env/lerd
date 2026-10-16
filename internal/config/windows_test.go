package config

import "testing"

// Windows 11's default blue, as reg.exe prints the AccentPalette value: eight
// RGBA shades from Light3 down to Dark3, then one Windows does not use.
const defaultAccentPalette = "99EBFF004CC2FF000091F8000078D4000067C000003E9200001A6800F7630C00"

func TestWindowsTheme_DefaultBlue(t *testing.T) {
	theme := windowsTheme(defaultAccentPalette)
	if theme == nil {
		t.Fatal("windowsTheme = nil for the default palette")
	}
	want := UITheme{
		ID:              WindowsThemeID,
		Name:            "Windows",
		Accent:          "#0067c0",
		AccentHover:     "#003e92",
		AccentDark:      "#4cc2ff",
		AccentHoverDark: "#99ebff",
		Source:          UIThemeSourceDesktop,
	}
	if *theme != want {
		t.Errorf("got %+v, want %+v", *theme, want)
	}
}

func TestWindowsTheme_RejectsWhatIsNotAPalette(t *testing.T) {
	for _, v := range []string{"", "99EBFF00", "not hex at all, but long enough to pass a length check.."} {
		if theme := windowsTheme(v); theme != nil {
			t.Errorf("windowsTheme(%q) = %+v, want nil", v, theme)
		}
	}
}

func TestParseRegBinary(t *testing.T) {
	out := "\r\nHKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Explorer\\Accent\r\n    AccentPalette    REG_BINARY    " + defaultAccentPalette + "\r\n\r\n"
	if got := parseRegBinary(out, "AccentPalette"); got != defaultAccentPalette {
		t.Errorf("got %q", got)
	}
	if got := parseRegBinary("ERROR: The system was unable to find the specified registry key or value.", "AccentPalette"); got != "" {
		t.Errorf("got %q from an error", got)
	}
}
