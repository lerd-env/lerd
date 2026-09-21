package config

import "testing"

// Omarchy keeps a state directory nothing else writes, and its themes carry
// surfaces a Plasma scheme on the same machine cannot improve on.
func TestCurrentDesktopPrefersOmarchy(t *testing.T) {
	writeKdeGlobals(t, "KDE", breezeDark)
	writeOmarchyTheme(t, "tokyo-night", `
mode = "dark"
accent = "#7aa2f7"
background = "#1a1b26"
lighter_background = "#24283b"
`)
	d := CurrentDesktop()
	if d.Theme == nil || d.Theme.ID != OmarchyThemeID {
		t.Fatalf("CurrentDesktop() = %+v, want the Omarchy theme", d.Theme)
	}
	if d.WatchDir != OmarchyCurrentDir() {
		t.Errorf("WatchDir = %q, want %q", d.WatchDir, OmarchyCurrentDir())
	}
}

func TestCurrentDesktopFallsBackToPlasma(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	writeKdeGlobals(t, "KDE", breezeDark)
	d := CurrentDesktop()
	if d.Theme == nil || d.Theme.ID != PlasmaThemeID {
		t.Fatalf("CurrentDesktop() = %+v, want the Plasma theme", d.Theme)
	}
}

func TestCurrentDesktopWithNoDesktopToFollow(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	writeKdeGlobals(t, "sway", "")
	d := CurrentDesktop()
	if d.Theme != nil || d.WatchDir != "" {
		t.Errorf("CurrentDesktop() = %+v, want nothing to follow", d)
	}
	if DesktopTheme() != nil {
		t.Error("DesktopTheme() lent a theme with no desktop to read")
	}
}
