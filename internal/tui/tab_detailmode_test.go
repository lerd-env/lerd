package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestSwitchTab_ResetsDetailMode guards against the Settings/System/Debug pane
// bleeding across tabs. Opening Settings on Sites leaves detailMode=detailSettings;
// switching to Services must drop back to detailSite so the Services detail column
// renders the selected service, not the stale global surface. The S/Y/D toggles are
// gated to the Sites tab, so a stuck pane would otherwise be unrecoverable there.
func TestSwitchTab_ResetsDetailMode(t *testing.T) {
	for _, mode := range []struct {
		key  rune
		want detailMode
	}{{'S', detailSettings}, {'Y', detailSystem}, {'D', detailDumps}} {
		m := NewModel("test")
		m.snap = fakeSnap()
		m.switchTab(tabSites)
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{mode.key}})
		m = next.(*Model)
		if m.detailMode != mode.want {
			t.Fatalf("%c on Sites should enter mode %d, got %d", mode.key, mode.want, m.detailMode)
		}
		m.switchTab(tabServices)
		if m.detailMode != detailSite {
			t.Fatalf("switching to Services after %c should reset to detailSite, got %d", mode.key, m.detailMode)
		}
		// And switching to the Dashboard must also clear it.
		m.switchTab(tabSites)
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{mode.key}})
		m = next.(*Model)
		m.switchTab(tabDashboard)
		if m.detailMode != detailSite {
			t.Fatalf("switching to Dashboard after %c should reset to detailSite, got %d", mode.key, m.detailMode)
		}
	}
}

// TestDashboardActions_NoSiteTarget guards against destructive/site actions firing
// against an invisible site on the Dashboard, where focus parks on paneDetail but no
// site list or cursor is shown. pause/unpause/restart/shell must no-op there rather
// than act on siteCursor (index 0 by default).
func TestDashboardActions_NoSiteTarget(t *testing.T) {
	actions := map[string]func(*Model) tea.Cmd{
		"stop":        (*Model).actionStop,
		"start":       (*Model).actionStart,
		"restart":     (*Model).actionRestart,
		"shell":       (*Model).actionShell,
		"pauseToggle": (*Model).actionPauseToggle,
		"openBrowser": (*Model).openInBrowserCmd,
	}
	for name, fn := range actions {
		m := NewModel("test")
		m.snap = fakeSnap()
		m.switchTab(tabDashboard)
		if cmd := fn(m); cmd != nil {
			t.Fatalf("action %q on Dashboard should no-op, got a command", name)
		}
		if m.status != "" {
			t.Fatalf("action %q on Dashboard should not set a status, got %q", name, m.status)
		}
	}
}

// TestDashboardEnter_NoSiteToggle guards the enter/space path: on the Dashboard
// it must not fall through to detailToggleSelected against the carried-over site
// (which would silently flip a worker / HTTPS / LAN share or open a picker modal).
func TestDashboardEnter_NoSiteToggle(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyRunes, Runes: []rune{' '}}} {
		m := NewModel("test")
		m.snap = fakeSnap()
		m.switchTab(tabDashboard)
		next, cmd := m.Update(key)
		m = next.(*Model)
		if cmd != nil {
			t.Fatalf("enter/space on Dashboard should no-op, got a command")
		}
		if m.pickerKind != kindInfo {
			t.Fatalf("enter/space on Dashboard should not open a picker")
		}
		if m.status != "" {
			t.Fatalf("enter/space on Dashboard should not set a status, got %q", m.status)
		}
	}
}

// TestSitesActions_StillTargetSite confirms the gating doesn't break the real path:
// on the Sites tab the same actions still resolve to the selected site.
func TestSitesActions_StillTargetSite(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.switchTab(tabSites)
	if cmd := m.actionStop(); cmd == nil {
		t.Fatalf("actionStop on Sites with a selected site should return a command")
	}
}
