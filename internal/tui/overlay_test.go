package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A modal draws over the dimmed screen instead of replacing it, and never
// changes the screen's size.
func TestModalDrawsOverTheScreen(t *testing.T) {
	m := sidebarModel()
	m.helpModalActive = true
	lines := strings.Split(m.render(), "\n")
	if len(lines) != m.height {
		t.Fatalf("%d lines, want %d", len(lines), m.height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.width {
			t.Fatalf("line %d is %d wide, want %d", i, w, m.width)
		}
	}
	screen := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(screen, "Keybindings") || !strings.Contains(screen, "SITES") {
		t.Fatalf("expected the help box over the sidebar still visible behind it:\n%s", screen)
	}
}
