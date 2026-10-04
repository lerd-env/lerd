package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/geodro/lerd/internal/siteinfo"
	"github.com/geodro/lerd/internal/stats"
)

func dashModel() *Model {
	m := NewModel("test")
	m.width, m.height = 160, 45
	m.snap = Snapshot{
		Sites: []siteinfo.EnrichedSite{
			{Name: "shop", Domains: []string{"shop.test"}, FPMRunning: true},
			{Name: "api", Domains: []string{"api.test"}, FPMRunning: true, HasQueueWorker: true, QueueFailing: true, Path: "/p/api"},
		},
		Services: []ServiceRow{
			{Name: "mysql", State: stateRunning},
			{Name: "queue-api", State: stateStopped, WorkerKind: "queue", WorkerSite: "api", WorkerPath: "/p/api"},
		},
		Status: StatusRow{TLD: "test", DNSOk: true, NginxRunning: true, WatcherRunning: true},
	}
	m.stats = stats.Snapshot{Available: true, TotalCPUPercent: 1, TotalMemBytes: 1 << 30, HostMemBytes: 32 << 30}
	m.activeTab = tabDashboard
	return m
}

func TestDashAlertsListCrashedWorkersAndDownCoreServices(t *testing.T) {
	m := dashModel()
	m.snap.Status.NginxRunning = false
	alerts := m.dashAlerts()
	if len(alerts) != 2 {
		t.Fatalf("got %d alerts, want nginx and the api worker: %+v", len(alerts), alerts)
	}
	if alerts[0].site != "" || !strings.Contains(alerts[0].title, "nginx") {
		t.Fatalf("core services come first, got %+v", alerts[0])
	}
	if alerts[1].site != "api" || alerts[1].worker != "queue" {
		t.Fatalf("second alert should be the api queue worker, got %+v", alerts[1])
	}
}

func TestDashAlertsEmptyWhenHealthy(t *testing.T) {
	m := dashModel()
	m.snap.Sites[1].QueueFailing = false
	if a := m.dashAlerts(); len(a) != 0 {
		t.Fatalf("healthy install should have no alerts, got %+v", a)
	}
	if out := ansi.Strip(m.renderDashboard(120, 40)); !strings.Contains(out, "Everything is running") {
		t.Fatalf("healthy dashboard should say so:\n%s", out)
	}
}

func TestDashEnterOpensTheAlertSite(t *testing.T) {
	m := dashModel()
	m.focusMain()
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(*Model)
	if m.activeTab != tabSites || m.currentSite().Name != "api" {
		t.Fatalf("enter on the worker alert should open api, got tab %v", m.activeTab)
	}
	if m.sideFocus || m.focus != paneDetail {
		t.Fatal("opening a site from the dashboard lands focus on its detail")
	}
}

func TestDashArrowsMoveBetweenAlerts(t *testing.T) {
	m := dashModel()
	m.snap.Status.WatcherRunning = false
	m.focusMain()
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.dashCursor != 1 {
		t.Fatalf("down should select the second alert, got %d", m.dashCursor)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.dashCursor != 1 {
		t.Fatalf("down past the last alert should stay put, got %d", m.dashCursor)
	}
}

// r is the one fix a card offers, and it has to target the crashed worker's
// own unit, not every worker on the machine.
func TestDashFixRestartsTheCrashedWorker(t *testing.T) {
	m := dashModel()
	a := m.dashAlerts()[0]
	if cmd := m.dashFix(a); cmd == nil {
		t.Fatal("fixing a crashed worker should return a command")
	}
	if !strings.Contains(m.status, "queue worker for api") {
		t.Fatalf("status should name the worker being restarted, got %q", m.status)
	}
}

func TestDashMarksContentBelowTheFold(t *testing.T) {
	m := dashModel()
	now := time.Now()
	for i := 0; i < 30; i++ {
		m.activity = append(m.activity, activityEvent{text: "event", at: now})
	}
	lines := strings.Split(m.renderDashboard(120, 40), "\n")
	if len(lines) != 40 {
		t.Fatalf("dashboard has %d lines, want 40", len(lines))
	}
	if !strings.Contains(ansi.Strip(lines[len(lines)-1]), "more below") {
		t.Fatalf("the last line should say more is below:\n%s", ansi.Strip(strings.Join(lines, "\n")))
	}
}

// Nothing is dropped for lack of room any more: the dashboard scrolls instead.
func TestDashScrollsToTheOldestRecentEntry(t *testing.T) {
	m := dashModel()
	m.focusMain()
	now := time.Now()
	for i := 0; i < activityCap; i++ {
		m.activity = append(m.activity, activityEvent{text: fmt.Sprintf("event-%02d", i), at: now})
	}
	m.renderDashboard(70, 20)
	for i := 0; i < 20; i++ {
		m.handleDashKey(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	out := ansi.Strip(m.renderDashboard(70, 20))
	if !strings.Contains(out, fmt.Sprintf("event-%02d", activityCap-1)) || strings.Contains(out, "more below") {
		t.Fatalf("pgdn should reach the oldest entry with nothing left below:\n%s", out)
	}
	for i, l := range strings.Split(m.renderDashboard(70, 20), "\n") {
		if w := ansi.StringWidth(l); w != 70 {
			t.Fatalf("line %d is %d wide, want 70", i, w)
		}
	}
}

func TestDashResourcesListsEveryContainer(t *testing.T) {
	m := dashModel()
	for _, n := range []string{"nginx", "mysql", "redis", "mailpit", "meilisearch", "php85-fpm"} {
		m.stats.Containers = append(m.stats.Containers, stats.ContainerStat{Name: "lerd-" + n, MemBytes: 10 << 20})
	}
	out := ansi.Strip(m.renderDashboard(160, 120))
	for _, n := range []string{"nginx", "mysql", "redis", "mailpit", "meilisearch", "php85-fpm"} {
		if !strings.Contains(out, n) {
			t.Errorf("resources leaves out %s", n)
		}
	}
}

func TestDashSystemCountsSleepingWorkers(t *testing.T) {
	m := dashModel()
	m.snap.Sites[1].QueueFailing = false
	m.snap.Services = append(m.snap.Services, ServiceRow{Name: "schedule-shop", State: stateSuspended, WorkerKind: "schedule", WorkerSite: "shop"})
	out := ansi.Strip(strings.Join(m.dashSystem(60), "\n"))
	if !strings.Contains(out, "0 running · 1 asleep") {
		t.Fatalf("idle-suspended workers should read as asleep:\n%s", out)
	}
}

func TestDashPanelPadsToTheRowHeight(t *testing.T) {
	m := dashModel()
	// 8 content rows plus padding, title and the blank under it.
	rows := m.dashPanel("System", 40, 8, []string{row(surf.s2, 36, sp("dns", nil))})
	if len(rows) != 12 {
		t.Fatalf("panel has %d rows, want 12 so it lines up with its neighbour", len(rows))
	}
	for i, r := range rows {
		if w := ansi.StringWidth(r); w != 40 {
			t.Fatalf("row %d is %d wide, want 40", i, w)
		}
	}
	if !strings.Contains(ansi.Strip(strings.Join(rows, "\n")), "SYSTEM") {
		t.Fatal("panel should carry its title")
	}
}

func TestDashWideStacksRecentUnderSystem(t *testing.T) {
	m := dashModel()
	m.snap.Sites[1].QueueFailing = false
	lines := strings.Split(ansi.Strip(m.renderDashboard(140, 60)), "\n")
	res, rec := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "RESOURCES") {
			res = i
			if !strings.Contains(l, "SYSTEM") {
				t.Fatalf("system should open on the same row as resources:\n%s", l)
			}
		}
		if c := strings.Index(l, "RECENT"); c >= 0 {
			rec = i
			if c < 70 {
				t.Fatalf("recent should sit in the second column, found at column %d:\n%s", c, l)
			}
		}
	}
	if res < 0 || rec <= res {
		t.Fatalf("recent should sit below system:\n%s", strings.Join(lines, "\n"))
	}
}

func TestDashSystemSplitsIntoTwoColumnsWhenItFits(t *testing.T) {
	m := dashModel()
	wide := strings.Split(ansi.Strip(strings.Join(m.dashSystem(90), "\n")), "\n")
	if len(wide) != 4 || !strings.Contains(wide[0], "dns") || !strings.Contains(wide[0], "autostart") {
		t.Fatalf("a wide panel should pair the rows into two columns:\n%s", strings.Join(wide, "\n"))
	}
	narrow := m.dashSystem(40)
	if len(narrow) != 8 {
		t.Fatalf("a narrow panel keeps one row per fact, got %d rows", len(narrow))
	}
}
