package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/geodro/lerd/internal/siteinfo"
)

func quickModel() *Model {
	m := NewModel("test")
	m.width, m.height = 160, 45
	m.snap = Snapshot{
		Sites: []siteinfo.EnrichedSite{
			{Name: "shop", Domains: []string{"shop.test"}, Path: "/p/shop", PHPVersion: "8.4", FPMRunning: true, HasQueueWorker: true, QueueFailing: true,
				Worktrees: []siteinfo.WorktreeInfo{{Branch: "feat", Domain: "feat.shop.test", Path: "/p/shop-feat"}}},
			{Name: "blog", Domains: []string{"blog.test"}, Path: "/p/blog", FPMRunning: true},
		},
		Services: []ServiceRow{
			{Name: "mysql", State: stateRunning},
			{Name: "redis", State: stateStopped},
			{Name: "queue-shop", State: stateStopped, WorkerKind: "queue", WorkerSite: "shop", WorkerPath: "/p/shop"},
		},
		Status: StatusRow{TLD: "test", DNSOk: true, NginxRunning: true, WatcherRunning: true},
	}
	return m
}

func typeKeys(m *Model, s string) *Model {
	for _, r := range s {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(*Model)
	}
	return m
}

func quickLabels(m *Model) []string {
	var out []string
	for _, a := range m.quickMatches() {
		out = append(out, a.label+" "+a.detail)
	}
	return out
}

func TestCtrlPOpensThePaletteAnywhere(t *testing.T) {
	m := quickModel()
	next, _ := m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	m = next.(*Model)
	if !m.quickActive {
		t.Fatal("ctrl+p should open the palette")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(*Model)
	if m.quickActive {
		t.Fatal("esc should close it")
	}
}

func TestPaletteOffersPlacesAndReversibleActions(t *testing.T) {
	m := quickModel()
	all := strings.Join(quickLabels(m), "\n")
	for _, want := range []string{"Open site shop.test", "Open worktree feat · shop.test", "Open service mysql",
		"Restart worker shop · queue", "Heal crashed workers", "Start service redis", "Stop service mysql", "Run a lerd command"} {
		if !strings.Contains(all, want) {
			t.Errorf("palette missing %q", want)
		}
	}
	for _, gone := range []string{"Remove", "Drop", "Unlink", "Restore"} {
		if strings.Contains(all, gone) {
			t.Errorf("palette must not offer %q, destructive actions stay in the CLI", gone)
		}
	}
}

func TestPaletteFuzzyMatchesAndRanksTighterFirst(t *testing.T) {
	m := quickModel()
	m.quickActive = true
	m = typeKeys(m, "blog")
	got := quickLabels(m)
	if len(got) == 0 || !strings.HasPrefix(got[0], "Open site blog.test") {
		t.Fatalf("typing blog should put the blog site first, got %v", got)
	}
	m.quickQuery = "zzqx"
	if got := quickLabels(m); len(got) != 0 {
		t.Fatalf("a query matching nothing should list nothing, got %v", got)
	}
}

func TestPaletteEnterOpensTheSite(t *testing.T) {
	m := quickModel()
	m.quickActive = true
	m = typeKeys(m, "open site blog")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(*Model)
	if m.quickActive || m.activeTab != tabSites || m.currentSite().Name != "blog" {
		t.Fatalf("enter should open blog and close the palette, tab %v active %v", m.activeTab, m.quickActive)
	}
}

func TestPaletteOpensAWorktreeTab(t *testing.T) {
	m := quickModel()
	m.quickActive = true
	m = typeKeys(m, "worktree feat")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.currentSite().Name != "shop" || m.siteWorktree(m.currentSite()) == nil {
		t.Fatal("opening a worktree should select its site and switch to its tab")
	}
}

func TestPaletteHandsOffToTheCommandPrompt(t *testing.T) {
	m := quickModel()
	m.quickActive = true
	m = typeKeys(m, "run a lerd")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(*Model)
	if m.quickActive || !m.paletteActive {
		t.Fatal("Run a lerd command should open the : prompt")
	}
}

func TestPaletteOverlayKeepsTheScreenSize(t *testing.T) {
	m := quickModel()
	m.quickActive = true
	lines := strings.Split(m.render(), "\n")
	if len(lines) != 45 {
		t.Fatalf("screen has %d lines, want 45", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 160 {
			t.Fatalf("line %d is %d wide", i, w)
		}
	}
	screen := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(screen, "Go to or do") || !strings.Contains(screen, "shop.test") {
		t.Fatal("the overlay should list the actions")
	}
}

func paletteText(m *Model) string {
	var lines []string
	for _, a := range m.quickActions() {
		lines = append(lines, a.label+" "+a.detail)
	}
	return strings.Join(lines, "\n")
}

// Everything is reachable from ctrl+p: every page, and every action on what is
// selected, the way opencode puts everything under one palette.
func TestPaletteReachesEveryPage(t *testing.T) {
	all := paletteText(quickModel())
	for _, want := range []string{"Go to Dashboard", "Go to Databases", "Go to PHP & Node", "Go to Settings", "Go to System",
		"Go to Debug window", "Go to dns", "Go to nginx", "Go to watcher", "Help", "Add a service preset"} {
		if !strings.Contains(all, want) {
			t.Errorf("palette missing page %q", want)
		}
	}
	if !strings.Contains(all, "Turn on") && !strings.Contains(all, "Turn off") {
		t.Error("palette should offer the settings toggles")
	}
}

func TestPaletteOffersTheSelectedSitesActions(t *testing.T) {
	m := quickModel()
	m.switchTab(tabSites)
	m.selectSiteByName("shop")
	all := paletteText(m)
	for _, want := range []string{"Restart site shop.test", "Pause site shop.test", "Open shell", "New worktree", "Show Logs", "Show Doctor",
		"Toggle keep awake", "Change PHP version", "Start or stop worker queue", "Add a domain"} {
		if !strings.Contains(all, want) {
			t.Errorf("palette missing site action %q", want)
		}
	}
}

func TestPaletteOffersTheSelectedServiceAndDatabaseActions(t *testing.T) {
	m := quickModel()
	m.switchTab(tabServices)
	m.selectServiceByName("mysql")
	all := paletteText(m)
	for _, want := range []string{"Pin service mysql", "Update service mysql", "Roll back service mysql", "Show Logs mysql"} {
		if !strings.Contains(all, want) {
			t.Errorf("palette missing service action %q", want)
		}
	}

	d := databasesModel()
	all = paletteText(d)
	for _, want := range []string{"Snapshot database shop", "Export database shop", "Create a database"} {
		if !strings.Contains(all, want) {
			t.Errorf("palette missing database action %q", want)
		}
	}
}

func TestPaletteSiteToggleRunsTheRow(t *testing.T) {
	m := quickModel()
	m.switchTab(tabSites)
	m.selectSiteByName("shop")
	for _, a := range m.quickActions() {
		if a.label == "Toggle keep awake" {
			if a.run(m) == nil || !strings.Contains(m.status, "keeping shop awake") {
				t.Fatalf("keep awake from the palette should pin the site, status %q", m.status)
			}
			return
		}
	}
	t.Fatal("no keep awake entry")
}

// Nothing has to be opened first: another site's controls are in the palette
// too, and running one selects that site.
func TestPaletteActsOnSitesThatAreNotSelected(t *testing.T) {
	m := quickModel()
	m.switchTab(tabDashboard)
	all := paletteText(m)
	for _, want := range []string{"Toggle HTTPS blog.test", "Restart site shop.test", "Start or stop worker queue shop.test", "Pin service redis"} {
		if !strings.Contains(all, want) {
			t.Errorf("palette missing %q from the dashboard", want)
		}
	}
	for _, a := range m.quickActions() {
		if a.label == "Toggle keep awake" && a.detail == "blog.test" {
			a.run(m)
			if m.activeTab != tabSites || m.currentSite().Name != "blog" || !strings.Contains(m.status, "blog") {
				t.Fatalf("running it should select blog and act on it, tab %v status %q", m.activeTab, m.status)
			}
			return
		}
	}
	t.Fatal("no keep awake entry for blog")
}

func TestPaletteWordsMatchInAnyOrder(t *testing.T) {
	m := quickModel()
	m.quickActive = true
	for _, q := range []string{"https blog", "blog https", "blo htt"} {
		m.quickQuery, m.quickCache = q, nil
		got := quickLabels(m)
		if len(got) == 0 || got[0] != "Toggle HTTPS blog.test" {
			t.Fatalf("%q should put Toggle HTTPS for blog first, got %v", q, got)
		}
	}
}
