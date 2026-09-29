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
			{Name: "shop", Domains: []string{"shop.test"}, Path: "/p/shop", FPMRunning: true, HasQueueWorker: true, QueueFailing: true,
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
