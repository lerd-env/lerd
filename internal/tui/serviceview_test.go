package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func serviceModel() *Model {
	m := NewModel("test")
	m.width, m.height = 160, 45
	m.snap = Snapshot{
		Services: []ServiceRow{
			{Name: "mysql", Version: "8.4", State: stateRunning, Dashboard: "http://localhost:8080"},
			{Name: "redis", Version: "7", State: stateStopped, Pinned: true, Custom: true},
		},
		Status: StatusRow{TLD: "test", DNSOk: true, NginxRunning: true, WatcherRunning: true},
	}
	m.switchTab(tabServices)
	m.svcCursor = 0
	m.focusMain()
	return m
}

func TestServiceViewFitsItsBox(t *testing.T) {
	m := serviceModel()
	lines := strings.Split(m.renderServiceView(120, 40), "\n")
	if len(lines) != 40 {
		t.Fatalf("service view has %d lines, want 40", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 120 {
			t.Fatalf("line %d is %d wide: %q", i, w, ansi.Strip(l))
		}
	}
}

func TestServiceHeaderCarriesTheFacts(t *testing.T) {
	m := serviceModel()
	out := ansi.Strip(m.renderServiceView(140, 40))
	for _, want := range []string{"Services  ›  mysql", "version 8.4", "running", "http://localhost:8080", "Overview", "Logs"} {
		if !strings.Contains(out, want) {
			t.Errorf("service view missing %q:\n%s", want, out)
		}
	}
	// The header owns these now, so the body must not repeat them.
	for _, gone := range []string{"state:", "unit:", "Actions", "💡"} {
		if strings.Contains(out, gone) {
			t.Errorf("body still repeats %q:\n%s", gone, out)
		}
	}
}

// Pinning keeps a service running when no site uses it; it has nothing to do
// with updates, which the old wording claimed.
func TestPinnedServiceSaysItStaysRunning(t *testing.T) {
	m := serviceModel()
	m.svcCursor = 1
	out := ansi.Strip(m.renderServiceView(140, 40))
	if !strings.Contains(out, "pinned") || strings.Contains(out, "auto-update") {
		t.Fatalf("pinned wording is wrong:\n%s", out)
	}
}

func TestServiceTabsSwitchWithDigitsAndL(t *testing.T) {
	m := serviceModel()
	m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if m.svcTab != svcTabLogs || !m.serviceLogsActive() {
		t.Fatal("2 should open the Logs tab and start the tail")
	}
	m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if m.svcTab != svcTabOverview || m.serviceLogsActive() {
		t.Fatal("1 should return to the Overview")
	}
	m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.svcTab != svcTabLogs || m.showLogs {
		t.Fatal("l on a service opens its Logs tab rather than the overlay")
	}
}

func TestPinKeyTogglesThePin(t *testing.T) {
	m := serviceModel()
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	m = next.(*Model)
	if cmd == nil || !strings.Contains(m.status, "pinning mysql") {
		t.Fatalf("P should pin mysql, status %q", m.status)
	}
	m.svcCursor = 1
	next, _ = m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	m = next.(*Model)
	if !strings.Contains(m.status, "unpinning redis") {
		t.Fatalf("P should unpin redis, status %q", m.status)
	}
}

func TestAddKeyOpensThePresetPalette(t *testing.T) {
	m := serviceModel()
	next, _ := m.Update(tea.KeyPressMsg{Code: 'A', Text: "A"})
	m = next.(*Model)
	if !m.paletteActive || m.paletteInput != "service preset " {
		t.Fatalf("A should open the palette on service preset, got %v %q", m.paletteActive, m.paletteInput)
	}
}
