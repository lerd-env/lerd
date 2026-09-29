package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func coreModel() *Model {
	m := NewModel("test")
	m.width, m.height = 160, 45
	m.snap = Snapshot{
		Services: []ServiceRow{{Name: "mysql", State: stateRunning}},
		Status:   StatusRow{TLD: "test", DNSOk: true, NginxRunning: true, WatcherRunning: false},
	}
	m.sideFocus = true
	return m
}

func TestCoreRowsCloseTheSidebar(t *testing.T) {
	m := coreModel()
	keys := strings.Join(sideKeys(m.sideItems()), " ")
	if !strings.HasSuffix(keys, "svc:mysql core:dns core:nginx core:watcher") {
		t.Fatalf("core rows should follow the services, got %s", keys)
	}
	out := ansi.Strip(strings.Join(m.renderSidebar(34, 30), "\n"))
	lines := strings.Split(out, "\n")
	tail := strings.Join(lines[len(lines)-5:], "\n")
	for _, want := range []string{"dns", "nginx", "watcher", "stopped"} {
		if !strings.Contains(tail, want) {
			t.Fatalf("footer should list %q:\n%s", want, tail)
		}
	}
}

func TestSelectingACoreRowOpensItsView(t *testing.T) {
	m := coreModel()
	m.sideMove(1 << 20) // last row: the watcher
	if m.activeTab != tabCore || m.coreName != "watcher" {
		t.Fatalf("expected the watcher view, got tab %v core %q", m.activeTab, m.coreName)
	}
	targets := m.currentLogTargets()
	if len(targets) != 1 || targets[0].Kind != kindJournal || targets[0].ID != "lerd-watcher" {
		t.Fatalf("the watcher logs come from its journal, got %+v", targets)
	}
	out := ansi.Strip(m.renderCoreView(120, 30))
	if !strings.Contains(out, "Core  ›  watcher") || !strings.Contains(out, "stopped") {
		t.Fatalf("core view should name it and its state:\n%s", out)
	}
	m.sideMove(-1)
	if targets := m.currentLogTargets(); targets[0].Kind != kindPodman || targets[0].ID != "lerd-nginx" {
		t.Fatalf("nginx logs come from its container, got %+v", targets)
	}
}

func TestCoreViewStartsLerd(t *testing.T) {
	m := coreModel()
	m.sideMove(1 << 20)
	m.focusMain()
	next, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = next.(*Model)
	if cmd == nil || !strings.Contains(m.status, "starting lerd") {
		t.Fatalf("s on a core view should start lerd, status %q", m.status)
	}
}

func TestCoreTabIsNotInTheCtrlArrowCycle(t *testing.T) {
	for _, tab := range orderedTabs {
		if tab == tabCore {
			t.Fatal("core views are reached from the sidebar, not the tab cycle")
		}
	}
}
