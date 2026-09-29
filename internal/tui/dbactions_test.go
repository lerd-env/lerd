package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/geodro/lerd/internal/siteinfo"
)

func dbActionModel(mode string) *Model {
	m := databasesModel()
	m.snap.Sites = []siteinfo.EnrichedSite{{Name: "shop", Domains: []string{"shop.test"}, Path: "/p/shop", AutoSnapshot: mode}}
	m.focusMain()
	m.focus = paneDatabases
	return m
}

func TestCreateKeyOpensThePaletteOnTheEngine(t *testing.T) {
	m := dbActionModel("")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(*Model)
	if !m.paletteActive || m.paletteInput != "db:create --service mysql " {
		t.Fatalf("c should open the palette on db:create for mysql, got %v %q", m.paletteActive, m.paletteInput)
	}
}

func TestExportKeyWritesNextToTheOwningSite(t *testing.T) {
	m := dbActionModel("")
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = next.(*Model)
	if cmd == nil || !strings.Contains(m.status, "/p/shop/shop.sql") {
		t.Fatalf("e should export shop into its site folder, status %q", m.status)
	}
}

func TestAutoSnapshotKeyFlipsTheOwningSite(t *testing.T) {
	m := dbActionModel("on")
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(*Model)
	if cmd == nil || !strings.Contains(m.status, "excluding shop") {
		t.Fatalf("a on a covered site should exclude it, status %q", m.status)
	}
	m = dbActionModel("off")
	next, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(*Model)
	if !strings.Contains(m.status, "including shop") {
		t.Fatalf("a on an excluded site should include it, status %q", m.status)
	}
}

func TestAutoSnapshotKeyNeedsAnOwner(t *testing.T) {
	m := dbActionModel("on")
	m.dbEngines[0].Databases[0].Owner.Domain = ""
	m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if !strings.Contains(m.status, "no site") {
		t.Fatalf("a database no site owns has no schedule to join, status %q", m.status)
	}
}
