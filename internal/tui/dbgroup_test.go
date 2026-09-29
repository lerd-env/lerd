package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/geodro/lerd/internal/dbview"
)

func testingPairModel() *Model {
	m := databasesModel()
	m.dbEngines[0].Databases = append(m.dbEngines[0].Databases,
		dbview.Entry{Name: "shop_testing", SizeBytes: 1 << 20, Owner: dbview.Owner{Domain: "shop.test"}},
		dbview.Entry{Name: "orphan_testing", SizeBytes: 1 << 10},
	)
	return m
}

// A "<db>_testing" database belongs with "<db>", as in the web UI, so it
// folds into that row instead of being listed a second time.
func TestTestingDatabaseFoldsIntoItsAppDatabase(t *testing.T) {
	m := testingPairModel()
	var names []string
	for _, r := range m.dbRows() {
		if r.database < 0 {
			continue
		}
		name := m.dbEngines[r.engine].Databases[r.database].Name
		if r.testing >= 0 {
			name += "+" + m.dbEngines[r.engine].Databases[r.testing].Name
		}
		names = append(names, name)
	}
	got := strings.Join(names, " ")
	if got != "shop+shop_testing shop_staging orphan_testing" {
		t.Fatalf("rows = %s", got)
	}
}

func TestFoldedRowAndDetailShowTheTestingDatabase(t *testing.T) {
	m := testingPairModel()
	list := ansi.Strip(m.renderDatabasesView(140, 30))
	if strings.Count(list, "shop_testing") != 1 || !strings.Contains(list, "+ testing") {
		t.Fatalf("the testing database should appear once, in the detail, and tag its row:\n%s", list)
	}
	if !strings.Contains(list, "Testing database") {
		t.Fatalf("the detail should describe the testing database:\n%s", list)
	}
}

func TestDatabasesViewFitsItsBox(t *testing.T) {
	m := testingPairModel()
	lines := strings.Split(m.renderDatabasesView(120, 30), "\n")
	if len(lines) != 30 {
		t.Fatalf("databases view has %d lines, want 30", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 120 {
			t.Fatalf("line %d is %d wide: %q", i, w, ansi.Strip(l))
		}
	}
}

func TestWheelScrollsTheDatabasesList(t *testing.T) {
	m := databasesModel()
	for i := 0; i < 40; i++ {
		m.dbEngines[0].Databases = append(m.dbEngines[0].Databases, dbview.Entry{Name: "db" + string(rune('a'+i%26)) + string(rune('a'+i/26))})
	}
	m.width, m.height = 150, 20
	_ = m.render()
	z := waitZone("pane:databases")
	if z.IsZero() {
		t.Fatal("databases list zone not registered")
	}
	next, _ := m.Update(tea.MouseWheelMsg{X: z.StartX + 2, Y: z.StartY + 2, Button: tea.MouseWheelDown})
	m = next.(*Model)
	if m.dbScroll == 0 {
		t.Fatal("wheel over the databases list should scroll it")
	}
}
