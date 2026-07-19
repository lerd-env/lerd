package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/geodro/lerd/internal/siteinfo"
)

func fakeSnap() Snapshot {
	return Snapshot{
		Sites: []siteinfo.EnrichedSite{
			{Name: "alpha", PHPVersion: "8.3", FPMRunning: true, HasQueueWorker: true, QueueRunning: true},
			{Name: "beta", PHPVersion: "8.2", FPMRunning: false, Paused: true},
		},
		Services: []ServiceRow{
			{Name: "mysql", State: stateRunning, SiteCount: 2},
			{Name: "redis", State: stateStopped, SiteCount: 1},
			{Name: "mailpit", State: statePaused, SiteCount: 0},
		},
		Status: StatusRow{
			TLD: "test", NginxRunning: true, DNSOk: true,
			PHPRunning: []string{"8.3"},
		},
	}
}

func TestMoveCursor_Sites_Bounds(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.activeTab = tabSites
	m.focus = paneSites

	m.moveCursor(1)
	if m.siteCursor != 1 {
		t.Fatalf("expected cursor 1, got %d", m.siteCursor)
	}
	m.moveCursor(5)
	if m.siteCursor != 1 {
		t.Fatalf("cursor should clamp at last site (1), got %d", m.siteCursor)
	}
	m.moveCursor(-99)
	if m.siteCursor != 0 {
		t.Fatalf("cursor should clamp at 0, got %d", m.siteCursor)
	}
}

func TestMoveCursor_Services_Bounds(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.activeTab = tabServices
	m.focus = paneServices

	m.moveCursor(2)
	if m.svcCursor != 2 {
		t.Fatalf("expected cursor 2, got %d", m.svcCursor)
	}
	m.moveCursor(99)
	if m.svcCursor != 2 {
		t.Fatalf("cursor should clamp at 2, got %d", m.svcCursor)
	}
}

func TestTabCyclesFocus(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.switchTab(tabSites)
	if m.focus != paneSites {
		t.Fatalf("initial focus should be sites, got %d", m.focus)
	}
	// On the Sites tab, focus cycles sites → detail → sites. Services now
	// live on their own tab, so they're no longer in the cycle.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(*Model)
	if m.focus != paneDetail {
		t.Fatalf("first tab should move focus to detail, got %d", m.focus)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(*Model)
	if m.focus != paneSites {
		t.Fatalf("second tab should wrap back to sites, got %d", m.focus)
	}
}

func TestTabSkipsDetailWhenNoSite(t *testing.T) {
	m := NewModel("test")
	m.snap = Snapshot{} // no sites
	m.switchTab(tabSites)
	// With no site selected there is no detail pane, so tab stays on the list.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(*Model)
	if m.focus != paneSites {
		t.Fatalf("tab with no sites should stay on the sites pane, got %d", m.focus)
	}
}

func TestSnapshotMsgClampsCursor(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.focus = paneSites
	m.siteCursor = 1

	shrunk := fakeSnap()
	shrunk.Sites = shrunk.Sites[:1]

	next, _ := m.Update(snapshotMsg{snap: shrunk})
	m = next.(*Model)
	if m.siteCursor != 0 {
		t.Fatalf("cursor should clamp when snapshot shrinks, got %d", m.siteCursor)
	}
}

func TestViewRendersCoreContent(t *testing.T) {
	m := NewModel("v1.0.0")
	m.snap = fakeSnap()
	m.width, m.height = 120, 30
	m.switchTab(tabSites)

	// Sites tab: tab-bar labels plus the sites list and detail.
	out := m.render()
	for _, want := range []string{"Dashboard", "Sites", "Services", "alpha", "beta"} {
		if !strings.Contains(out, want) {
			t.Fatalf("sites view missing %q\n---\n%s", want, out)
		}
	}
	// Services tab renders the services list.
	m.switchTab(tabServices)
	out = m.render()
	for _, want := range []string{"mysql", "redis"} {
		if !strings.Contains(out, want) {
			t.Fatalf("services view missing %q\n---\n%s", want, out)
		}
	}
}

func TestViewTooSmall(t *testing.T) {
	m := NewModel("v1.0.0")
	m.snap = fakeSnap()
	m.width, m.height = 40, 10
	out := m.render()
	if !strings.Contains(out, "too small") {
		t.Fatalf("expected too-small banner, got: %s", out)
	}
}

func TestCurrentSiteAndService(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()

	if m.currentSite().Name != "alpha" {
		t.Fatal("expected alpha at cursor 0")
	}
	m.siteCursor = 1
	if m.currentSite().Name != "beta" {
		t.Fatal("expected beta at cursor 1")
	}

	// Services render sorted by name by default, so the first row is mailpit,
	// not mysql (the snapshot insertion order).
	if m.currentService().Name != "mailpit" {
		t.Fatalf("expected mailpit at cursor 0 (name sort), got %s", m.currentService().Name)
	}
}

func TestFilterNarrowsSitesList(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.siteFilter = "bet"
	visible := m.visibleSites()
	if len(visible) != 1 || visible[0].Name != "beta" {
		t.Fatalf("expected filter 'bet' to keep only beta, got %+v", names(visible))
	}
}

func TestSortSitesByStatusRunningFirst(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.siteSort = siteSortStatus
	got := m.visibleSites()
	if got[0].Name != "alpha" {
		t.Fatalf("expected running site (alpha) first under status sort, got %s", got[0].Name)
	}
}

func names(ss []siteinfo.EnrichedSite) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Name
	}
	return out
}

func TestFormatActionOK(t *testing.T) {
	got := formatAction(ActionResult{Summary: "lerd service start redis"})
	if !strings.HasPrefix(got, "✓") {
		t.Fatalf("expected ok prefix, got %q", got)
	}
}

func TestFormatActionError(t *testing.T) {
	got := formatAction(ActionResult{
		Summary: "lerd service start redis",
		Err:     errors.New("exit 1"),
		Detail:  "boom\ntrace",
	})
	if !strings.Contains(got, "boom") || strings.Contains(got, "trace") {
		t.Fatalf("expected first-line error only, got %q", got)
	}
}

func TestViewportScrollsToKeepCursorVisible(t *testing.T) {
	rows := make([]string, 20)
	for i := range rows {
		rows[i] = "row"
	}
	scroll := 0
	visible := viewport(rows, 15, 5, &scroll)
	if len(visible) != 5 {
		t.Fatalf("expected 5 visible rows, got %d", len(visible))
	}
	if scroll != 11 {
		t.Fatalf("expected scroll=11 to keep cursor visible, got %d", scroll)
	}
}
