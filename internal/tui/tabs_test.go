package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/geodro/lerd/internal/siteinfo"
	zone "github.com/lrstanley/bubblezone"
)

// TestMain initialises the global bubblezone manager so View() (which marks
// clickable regions) doesn't panic outside of Run, and points the whole package at
// a temp XDG root. The TUI's load path reads and writes the site registry, and a
// test driving it without isolation writes the developer's own sites.yaml; that
// has already emptied one. Isolating here covers every test in the package rather
// than relying on each to remember.
func TestMain(m *testing.M) {
	zone.NewGlobal()
	subprocessesAllowed = false
	dir, err := os.MkdirTemp("", "lerd-tui-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "tui tests: temp XDG root:", err)
		os.Exit(1)
	}
	os.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// waitZone polls for a zone to register. zone.Scan hands positions to a
// background worker, so a Get immediately after a render can race ahead of it;
// this gives the worker a bounded window to catch up.
func waitZone(id string) *zone.ZoneInfo {
	for i := 0; i < 200; i++ {
		if z := zone.Get(id); !z.IsZero() {
			return z
		}
		time.Sleep(time.Millisecond)
	}
	return zone.Get(id)
}

func TestNextTab_CyclesBothDirections(t *testing.T) {
	m := NewModel("test")
	m.activeTab = tabDashboard
	if got := m.nextTab(+1); got != tabSites {
		t.Fatalf("dashboard +1 should be sites, got %d", got)
	}
	m.activeTab = tabServices
	if got := m.nextTab(+1); got != tabDashboard {
		t.Fatalf("services +1 should wrap to dashboard, got %d", got)
	}
	m.activeTab = tabDashboard
	if got := m.nextTab(-1); got != tabServices {
		t.Fatalf("dashboard -1 should wrap to services, got %d", got)
	}
}

func TestSwitchTab_ParksFocus(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.switchTab(tabServices)
	if m.activeTab != tabServices || m.focus != paneServices {
		t.Fatalf("switch to services should focus services pane, got tab=%d focus=%d", m.activeTab, m.focus)
	}
	m.switchTab(tabDashboard)
	if m.activeTab != tabDashboard || m.focus != paneDetail {
		t.Fatalf("switch to dashboard should park focus on detail, got tab=%d focus=%d", m.activeTab, m.focus)
	}
}

func TestNextFocus_DashboardStaysOnDetail(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.activeTab = tabDashboard
	if got := m.nextFocus(+1); got != paneDetail {
		t.Fatalf("dashboard tab has no list panes, expected detail, got %d", got)
	}
}

func TestMouseClick_SwitchesTab(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.width, m.height = 150, 40
	_ = m.View() // register zones

	z := waitZone("tab:" + tabServices.label())
	if z.IsZero() {
		t.Fatalf("services tab zone not registered after render")
	}
	msg := tea.MouseMsg{X: z.StartX, Y: z.StartY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	next, _ := m.Update(msg)
	m = next.(*Model)
	if m.activeTab != tabServices {
		t.Fatalf("clicking the Services tab should switch to it, got %d", m.activeTab)
	}
}

func TestMouseClick_SelectsSiteRow(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.activeTab = tabSites
	m.focus = paneSites
	m.width, m.height = 150, 40
	_ = m.View()

	z := waitZone("site:1")
	if z.IsZero() {
		t.Fatalf("second site row zone not registered after render")
	}
	msg := tea.MouseMsg{X: z.StartX, Y: z.StartY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	next, _ := m.Update(msg)
	m = next.(*Model)
	if m.siteCursor != 1 {
		t.Fatalf("clicking the second site row should select index 1, got %d", m.siteCursor)
	}
}

func TestMouseClick_IgnoresNonLeftPress(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.width, m.height = 150, 40
	_ = m.View()
	z := zone.Get("tab:" + tabServices.label())
	// Motion (not a press) must not switch tabs.
	msg := tea.MouseMsg{X: z.StartX, Y: z.StartY, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft}
	next, _ := m.Update(msg)
	m = next.(*Model)
	if m.activeTab == tabServices {
		t.Fatalf("motion event should not switch tabs")
	}
}

func TestRenderDashboardGrid_HasAllSixCards(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	out := m.renderDashboardGrid(150, 30)
	for _, title := range []string{"Sites", "Services", "Workers", "System Health", "Resources", "Lerd"} {
		if !strings.Contains(out, title) {
			t.Fatalf("dashboard grid missing %q card:\n%s", title, out)
		}
	}
}

func TestDashboardClick_JumpsToSiteTab(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.activeTab = tabDashboard
	m.width, m.height = 150, 40
	_ = m.View() // register dashboard row zones

	z := waitZone("dashsite:1")
	if z.IsZero() {
		t.Fatalf("dashsite row zone not registered after render")
	}
	msg := tea.MouseMsg{X: z.StartX, Y: z.StartY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	next, _ := m.Update(msg)
	m = next.(*Model)
	if m.activeTab != tabSites {
		t.Fatalf("clicking a dashboard site should switch to the Sites tab, got %d", m.activeTab)
	}
	if s := m.currentSite(); s == nil || s.Name != "beta" {
		t.Fatalf("expected the clicked site (beta) selected, got %+v", s)
	}
}

func TestDashboardTab_CyclesCardFocus(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.activeTab = tabDashboard
	m.dashFocus = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(*Model)
	if m.dashFocus != 1 {
		t.Fatalf("tab on dashboard should advance card focus, got %d", m.dashFocus)
	}
}

func TestDiffSnapshots_DetectsChanges(t *testing.T) {
	now := time.Now()
	// Newly linked site.
	if evs := diffSnapshots(Snapshot{}, fakeSnap(), now); len(evs) == 0 {
		t.Fatalf("expected events for newly linked sites")
	}
	// Pausing a site emits a paused event.
	prev := fakeSnap()
	cur := fakeSnap()
	cur.Sites[0].Paused = true
	cur.Sites[0].FPMRunning = false
	got := diffSnapshots(prev, cur, now)
	found := false
	for _, e := range got {
		if strings.Contains(e.text, "paused") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a paused event, got %+v", got)
	}
}

func TestRecordActivity_CapsRing(t *testing.T) {
	m := NewModel("test")
	m.prevSnap = &Snapshot{}
	// Each record diffs a snapshot with many fresh sites against the previous
	// (empty) baseline; after the first the baseline matches, so drive churn by
	// alternating an empty snapshot with a populated one.
	for i := 0; i < activityCap+5; i++ {
		if i%2 == 0 {
			m.recordActivity(fakeSnap(), time.Now())
		} else {
			m.recordActivity(Snapshot{}, time.Now())
		}
	}
	if len(m.activity) > activityCap {
		t.Fatalf("activity ring should be capped at %d, got %d", activityCap, len(m.activity))
	}
}

func TestSiteLogsActive_GatedToSitesLogsTab(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.siteTab = tabSiteLogs

	m.activeTab = tabDashboard
	if m.siteLogsActive() {
		t.Fatal("site logs should be inactive on the Dashboard tab")
	}
	m.activeTab = tabServices
	if m.siteLogsActive() {
		t.Fatal("site logs should be inactive on the Services tab")
	}
	m.activeTab = tabSites
	m.siteTab = tabSiteOverview
	if m.siteLogsActive() {
		t.Fatal("site logs should be inactive on a non-Logs site tab")
	}
	m.siteTab = tabSiteLogs
	if !m.siteLogsActive() {
		t.Fatal("site logs should be active on the Sites Logs tab")
	}
	// A pane swap (S / Y / D) takes the detail column, so the tail steps aside.
	m.detailMode = detailSystem
	if m.siteLogsActive() {
		t.Fatal("site logs should be inactive while a pane swap owns the detail column")
	}
}

func TestSiteTabs_LogsIsSecondAndReachableByNumber(t *testing.T) {
	tabs := availableSiteTabs(&siteinfo.EnrichedSite{Name: "alpha"})
	if tabs[1] != tabSiteLogs {
		t.Fatalf("Logs should be the second site tab, got %v", tabs)
	}
	// Every tab must be reachable by its 1-based number, so the strip's rendered
	// index and the working key can't drift.
	m := NewModel("test")
	m.snap = fakeSnap()
	m.activeTab = tabSites
	for i, want := range tabs {
		if want == tabSiteDoctor {
			// Doctor kicks off a real check run; its routing is covered separately.
			continue
		}
		m.selectSiteTab(i + 1)
		if m.siteTab != want {
			t.Fatalf("key %d selected %v, want %v", i+1, m.siteTab, want)
		}
	}
}

func TestLKey_SelectsLogsTabOnSites(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.activeTab = tabSites

	m.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if m.siteTab != tabSiteLogs {
		t.Fatalf("l on the Sites tab should select the Logs tab, got %v", m.siteTab)
	}
	if m.showLogs {
		t.Fatal("l on the Sites tab should not also open the full-width overlay")
	}
	if !m.logsInDetail() {
		t.Fatal("the Logs tab should report the tail as showing in the detail column")
	}
}

func TestLKey_ClosesAnOverlayCarriedInFromAnotherTab(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	// `l` on the Services tab sets showLogs, though the pane stays hidden behind
	// the service detail's own tail. Walking onto the Sites tab then reveals it.
	m.activeTab = tabServices
	m.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if !m.showLogs {
		t.Fatal("l on the Services tab should set showLogs")
	}
	m.activeTab = tabSites

	// l must close the pane rather than select the tab underneath it, or the
	// overlay would be stuck open with no key that dismisses it.
	m.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if m.showLogs {
		t.Fatal("l on the Sites tab should close an overlay carried in from another tab")
	}
	m.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if m.siteTab != tabSiteLogs {
		t.Fatal("with no overlay open, l should select the Logs tab")
	}
}

func TestCycleLogTarget_WorksOnLogsTabWithoutTheOverlay(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	// alpha has FPM plus a queue worker, so it has more than one log source.
	m.activeTab = tabSites
	m.siteTab = tabSiteLogs
	if n := len(m.currentLogTargets()); n < 2 {
		t.Fatalf("fixture needs >1 log target to exercise cycling, got %d", n)
	}
	// The Logs tab shows the tail without the full-width `l` overlay, so cycling
	// must not be gated on showLogs.
	if m.showLogs {
		t.Fatal("Logs tab should not set showLogs")
	}
	m.cycleLogTarget(1)
	if m.logCursor != 1 {
		t.Fatalf("] should advance the log target on the Logs tab, cursor stayed at %d", m.logCursor)
	}
	m.cycleLogTarget(-1)
	if m.logCursor != 0 {
		t.Fatalf("[ should step the log target back, cursor at %d", m.logCursor)
	}
}

func TestRenderLogs_NoSourceEmptyState(t *testing.T) {
	m := NewModel("test")
	// A site with no container and no workers has nothing to tail.
	m.snap = Snapshot{Sites: []siteinfo.EnrichedSite{{Name: "idle"}}}
	m.activeTab = tabSites
	m.siteTab = tabSiteLogs

	out := stripANSI(m.renderDetailColumn(80, 16, true))
	if strings.Contains(out, "Logs ·") {
		t.Fatalf("no-source header should not dangle a separator:\n%s", out)
	}
	if !strings.Contains(out, "no log source for this site") {
		t.Fatalf("expected a no-source header:\n%s", out)
	}
	if strings.Contains(out, "waiting for output") {
		t.Fatalf("no-source site should not read as waiting on a live tail:\n%s", out)
	}
}

func TestRenderLogs_LogsTabKeepsTabStrip(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.activeTab = tabSites
	m.siteTab = tabSiteLogs

	out := stripANSI(m.renderDetailColumn(80, 20, true))
	if !strings.Contains(out, "[2] Logs") {
		t.Fatalf("Logs tab should keep the site tab strip on top:\n%s", out)
	}
	if !strings.Contains(out, "Logs ·") {
		t.Fatalf("Logs tab should render the tail header:\n%s", out)
	}
}

func TestWheel_ScrollsSitesPaneNotSelection(t *testing.T) {
	m := NewModel("test")
	sites := make([]siteinfo.EnrichedSite, 40)
	for i := range sites {
		sites[i] = siteinfo.EnrichedSite{Name: fmt.Sprintf("site%02d", i)}
	}
	m.snap = Snapshot{Sites: sites}
	m.switchTab(tabSites)
	m.width, m.height = 150, 20
	_ = m.View()

	z := waitZone("pane:sites")
	if z.IsZero() {
		t.Fatalf("sites pane zone not registered after render")
	}
	msg := tea.MouseMsg{X: z.StartX, Y: z.StartY, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown}
	next, _ := m.Update(msg)
	m = next.(*Model)
	if m.siteScroll == 0 {
		t.Fatalf("wheel down over the sites pane should scroll the viewport, siteScroll still 0")
	}
	if m.siteCursor != 0 {
		t.Fatalf("wheel must not move the selection, got cursor %d", m.siteCursor)
	}
}

func TestView_RendersEachTab(t *testing.T) {
	for _, tab := range orderedTabs {
		m := NewModel("test")
		m.snap = fakeSnap()
		m.width, m.height = 150, 40
		m.activeTab = tab
		out := m.View()
		// The tab bar labels are always present regardless of the active tab.
		for _, label := range []string{"Dashboard", "Sites", "Services"} {
			if !strings.Contains(out, label) {
				t.Fatalf("tab %d view missing tab label %q", tab, label)
			}
		}
	}
}
