package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteinfo"
)

func sidebarModel() *Model {
	m := NewModel("test")
	m.width, m.height = 160, 45
	m.snap = Snapshot{
		Sites: []siteinfo.EnrichedSite{
			{Name: "shop", Domains: []string{"shop.test"}, FPMRunning: true, FrameworkLabel: "Laravel 12"},
			{Name: "api", Domains: []string{"api.test"}, FPMRunning: true, HasQueueWorker: true, QueueFailing: true},
			{Name: "blog", Domains: []string{"blog.test"}, Paused: true},
			{Name: "loose", Domains: []string{"loose.test"}, FPMRunning: true},
		},
		Services: []ServiceRow{
			{Name: "mysql", Version: "8.4", State: stateRunning},
			{Name: "queue-api", State: stateRunning, WorkerKind: "queue", WorkerSite: "api"},
			{Name: "redis", Version: "7", State: stateStopped},
		},
		Workspaces: []config.Workspace{
			{Name: "studio", Sites: []string{"blog"}},
			{Name: "acme", Sites: []string{"shop", "api"}},
		},
		Status: StatusRow{TLD: "test", DNSOk: true, NginxRunning: true, WatcherRunning: true},
	}
	m.activeTab = tabDashboard
	m.sideFocus = true
	return m
}

func sideKeys(items []sideItem) []string {
	var out []string
	for _, it := range items {
		if it.selectable() {
			out = append(out, it.key)
		}
	}
	return out
}

func TestSideItemsGroupSitesByWorkspaceInConfigOrder(t *testing.T) {
	m := sidebarModel()
	got := strings.Join(sideKeys(m.sideItems()), " ")
	want := "dash dbs rt ws:studio site:blog ws:acme site:api site:shop site:loose svc:mysql svc:redis core:dns core:nginx core:watcher"
	if got != want {
		t.Fatalf("sidebar order\n got %s\nwant %s", got, want)
	}
}

func TestSideItemsHideSitesOfCollapsedWorkspace(t *testing.T) {
	m := sidebarModel()
	m.collapsedWS = map[string]bool{"acme": true}
	got := strings.Join(sideKeys(m.sideItems()), " ")
	if strings.Contains(got, "site:api") || strings.Contains(got, "site:shop") || !strings.Contains(got, "ws:acme") {
		t.Fatalf("collapsed workspace still lists its sites: %s", got)
	}
}

// A crashed worker has to be visible from the collapsed workspace row, or the
// failure hides behind a fold the user has no reason to open.
func TestWorkspaceRollupCountsFailingSites(t *testing.T) {
	m := sidebarModel()
	failing, paused := m.workspaceRollup("acme")
	if failing != 1 || paused != 0 {
		t.Fatalf("acme rollup = %d failing, %d paused; want 1, 0", failing, paused)
	}
	failing, paused = m.workspaceRollup("studio")
	if failing != 0 || paused != 1 {
		t.Fatalf("studio rollup = %d failing, %d paused; want 0, 1", failing, paused)
	}
}

func TestSideMoveSelectsSiteAndSwitchesTab(t *testing.T) {
	m := sidebarModel()
	m.sideMove(4) // dash -> dbs -> rt -> ws:studio -> site:blog
	if m.activeTab != tabSites {
		t.Fatalf("activeTab = %v, want sites", m.activeTab)
	}
	if s := m.currentSite(); s == nil || s.Name != "blog" {
		t.Fatalf("current site = %+v, want blog", s)
	}
	m.sideMove(6) // blog -> acme, api, shop, loose, mysql, redis
	if m.activeTab != tabServices || m.currentService().Name != "redis" {
		t.Fatalf("moving past the sites should land on the last service, got tab %v", m.activeTab)
	}
}

func TestSideMoveStopsOnWorkspaceWithoutChangingTab(t *testing.T) {
	m := sidebarModel()
	m.sideMove(3)
	if m.sideKey != "ws:studio" || m.activeTab != tabDashboard {
		t.Fatalf("sideKey = %q tab %v, want ws:studio on the dashboard", m.sideKey, m.activeTab)
	}
}

func TestSideActivateTogglesWorkspaceAndOpensSite(t *testing.T) {
	m := sidebarModel()
	m.sideMove(3)
	m.sideActivate()
	if !m.collapsedWS["studio"] {
		t.Fatal("enter on a workspace should collapse it")
	}
	m.sideActivate()
	if m.collapsedWS["studio"] {
		t.Fatal("enter again should expand it")
	}
	m.sideMove(1)
	m.sideActivate()
	if m.sideFocus || m.focus != paneDetail {
		t.Fatalf("enter on a site should hand focus to the detail, got sideFocus=%v focus=%v", m.sideFocus, m.focus)
	}
}

// Selection made elsewhere (ctrl+arrows, a dashboard jump, the palette) must
// show in the sidebar, so the key follows the model state.
func TestSideKeyFollowsExternalSelection(t *testing.T) {
	m := sidebarModel()
	m.switchTab(tabServices)
	m.svcCursor = 0
	m.syncSideKey()
	if m.sideKey != "svc:mysql" {
		t.Fatalf("sideKey = %q, want svc:mysql", m.sideKey)
	}
}

func TestRenderSidebarFitsItsBox(t *testing.T) {
	m := sidebarModel()
	lines := m.renderSidebar(34, 30)
	if len(lines) != 30 {
		t.Fatalf("sidebar has %d lines, want 30", len(lines))
	}
	joined := ""
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 34 {
			t.Fatalf("line %d width %d, want 34: %q", i, w, ansi.Strip(l))
		}
		joined += ansi.Strip(l) + "\n"
	}
	for _, want := range []string{"Dashboard", "SITES", "studio", "shop.test", "SERVICES", "mysql", "dns"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("sidebar missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "queue-api") {
		t.Fatal("workers belong to their site, not the services list")
	}
}

func TestRenderFullScreenFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{160, 45}, {110, 34}, {80, 24}} {
		m := sidebarModel()
		m.width, m.height = size[0], size[1]
		lines := strings.Split(m.render(), "\n")
		if len(lines) != size[1] {
			t.Fatalf("%dx%d: %d lines", size[0], size[1], len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > size[0] {
				t.Fatalf("%dx%d: line %d is %d wide", size[0], size[1], i, w)
			}
		}
	}
}
