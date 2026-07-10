package tui

import (
	"strings"
	"testing"
)

// The help pane is the only place the TUI advertises its sort modes, so it has
// to name every mode the "o" key actually cycles through.
func TestHelpNamesEverySiteSortMode(t *testing.T) {
	m := NewModel("test")
	lines := helpContentLines(m, 80)
	out := strings.Join(lines, "\n")

	for mode := siteSortMode(0); mode < siteSortModes; mode++ {
		if !strings.Contains(out, mode.label()) {
			t.Errorf("help never mentions the %q sort mode:\n%s", mode.label(), out)
		}
	}
}

func TestHelpNamesEveryServiceSortMode(t *testing.T) {
	m := NewModel("test")
	out := strings.Join(helpContentLines(m, 80), "\n")

	for _, mode := range []svcSortMode{svcSortName, svcSortStatus, svcSortUsage} {
		if !strings.Contains(out, mode.label()) {
			t.Errorf("help never mentions the %q service sort mode:\n%s", mode.label(), out)
		}
	}
}

// The key column is padded to 18 cells and clipped, so a row whose key outgrows
// it would silently lose text. truncatePlain counts runes, not bytes.
func TestHelpRowsFitTheKeyColumn(t *testing.T) {
	for _, sec := range helpReference {
		for _, row := range sec.rows {
			if n := len([]rune(row[0])); n > 18 {
				t.Errorf("section %q: key %q is %d runes, the column truncates at 18", sec.title, row[0], n)
			}
		}
	}
}
