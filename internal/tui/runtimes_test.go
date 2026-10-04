package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/geodro/lerd/internal/siteinfo"
)

func runtimesModel(t *testing.T) *Model {
	t.Helper()
	restore := stubRuntimes(runtimeFacts{php: []string{"8.3", "8.4"}, node: []string{"20", "22"}, defaultPHP: "8.4", defaultNode: "22", xdebug: map[string]bool{"8.3": true}})
	t.Cleanup(restore)
	m := NewModel("test")
	m.width, m.height = 160, 45
	m.snap = Snapshot{
		Sites: []siteinfo.EnrichedSite{
			{Name: "shop", Domains: []string{"shop.test"}, PHPVersion: "8.4", NodeVersion: "22"},
			{Name: "legacy", Domains: []string{"legacy.test"}, PHPVersion: "8.3", NodeVersion: "20"},
		},
		Status: StatusRow{PHPRunning: []string{"8.4"}},
	}
	m.switchTab(tabRuntimes)
	m.focusMain()
	return m
}

func TestRuntimesListEveryInstalledVersion(t *testing.T) {
	m := runtimesModel(t)
	var got []string
	for _, r := range m.runtimeRows() {
		got = append(got, r.kind+" "+r.version)
	}
	if strings.Join(got, ",") != "php 8.3,php 8.4,node 20,node 22" {
		t.Fatalf("rows = %v", got)
	}
}

func TestRuntimeDetailShowsStateAndSites(t *testing.T) {
	m := runtimesModel(t)
	out := ansi.Strip(m.renderRuntimesView(140, 40))
	for _, want := range []string{"PHP & Node", "PHP 8.3", "FPM", "stopped", "Xdebug", "legacy.test"} {
		if !strings.Contains(out, want) {
			t.Errorf("runtimes view missing %q:\n%s", want, out)
		}
	}
}

func TestRuntimeKeys(t *testing.T) {
	m := runtimesModel(t)
	m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if !strings.Contains(m.status, "PHP 8.3 the default") {
		t.Fatalf("d should make PHP 8.3 the default, status %q", m.status)
	}
	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !strings.Contains(m.status, "xdebug off PHP 8.3") {
		t.Fatalf("x should turn Xdebug off for 8.3, status %q", m.status)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if !strings.Contains(m.status, "Node 20 the default") {
		t.Fatalf("d on a Node row should use node:use, status %q", m.status)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = next.(*Model)
	if !m.paletteActive || m.paletteInput != "node:install " {
		t.Fatalf("i on Node should open node:install, got %q", m.paletteInput)
	}
}

func TestSidebarHasARuntimesEntry(t *testing.T) {
	m := runtimesModel(t)
	if keys := strings.Join(sideKeys(m.sideItems()), " "); !strings.HasPrefix(keys, "dash dbs rt ") {
		t.Fatalf("PHP & Node should follow Databases, got %s", keys)
	}
}

// stubRuntimes swaps the installed-runtime lookup for fixed facts, so the tests
// never read the machine's PHP or Node installs.
func stubRuntimes(f runtimeFacts) func() {
	old := loadRuntimeFacts
	loadRuntimeFacts = func() runtimeFacts { return f }
	return func() { loadRuntimeFacts = old }
}

func TestMouseClick_RuntimeRowSelectsIt(t *testing.T) {
	m := runtimesModel(t)
	m.width, m.height = 150, 40
	if len(m.runtimeRows()) < 2 {
		t.Fatal("fixture needs two runtime rows")
	}
	_ = m.render()

	z := waitZone("rt:1")
	if z.IsZero() {
		t.Fatalf("second runtime row not registered after render")
	}
	next, _ := m.Update(tea.MouseClickMsg{X: z.StartX, Y: z.StartY, Button: tea.MouseLeft})
	if next.(*Model).rtCursor != 1 {
		t.Fatalf("clicking the second runtime row left the cursor at %d", next.(*Model).rtCursor)
	}
}
