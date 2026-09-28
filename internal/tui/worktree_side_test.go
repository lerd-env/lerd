package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/geodro/lerd/internal/siteinfo"
)

func worktreeModel() *Model {
	m := NewModel("test")
	m.width, m.height = 160, 45
	m.snap = Snapshot{
		Sites: []siteinfo.EnrichedSite{
			{Name: "shop", Domains: []string{"shop.test"}, Path: "/p/shop", FPMRunning: true, PHPVersion: "8.4",
				Worktrees: []siteinfo.WorktreeInfo{
					{Branch: "feat-cart", Path: "/p/shop/.worktrees/feat-cart", PHPVersion: "8.4",
						FrameworkWorkers: []siteinfo.WorkerInfo{{Name: "vite", Failing: true}}},
					{Branch: "fix-tax", Path: "/p/shop/.worktrees/fix-tax"},
				}},
			{Name: "blog", Domains: []string{"blog.test"}, Path: "/p/blog", FPMRunning: true},
		},
		Status: StatusRow{TLD: "test", DNSOk: true, NginxRunning: true, WatcherRunning: true},
	}
	return m
}

func TestSidebarNestsWorktreesUnderTheirSite(t *testing.T) {
	m := worktreeModel()
	got := strings.Join(sideKeys(m.sideItems()), " ")
	want := "dash dbs site:blog site:shop wt:shop/feat-cart wt:shop/fix-tax"
	if !strings.HasPrefix(got, want) {
		t.Fatalf("sidebar order\n got %s\nwant prefix %s", got, want)
	}
	out := ansi.Strip(strings.Join(m.renderSidebar(34, 40), "\n"))
	if !strings.Contains(out, "├ ✖ feat-cart") || !strings.Contains(out, "└ fix-tax") {
		t.Fatalf("worktree row not drawn:\n%s", out)
	}
}

func TestWorktreeRowShowsItsCrashedWorker(t *testing.T) {
	m := worktreeModel()
	if g := worktreeGlyph(m.snap.Sites[0].Worktrees[0]); strings.TrimSpace(g.t) != glyphFailing {
		t.Fatalf("a worktree with a crashed worker should read failing, got %q", g.t)
	}
	if g := worktreeGlyph(m.snap.Sites[0].Worktrees[1]); strings.TrimSpace(g.t) == glyphFailing {
		t.Fatal("a healthy worktree should not read failing")
	}
}

// Opening a worktree lands on its rows in the parent's Overview, which is
// where its workers, database, LAN, PHP and Node controls live.
func TestOpeningAWorktreeSelectsItsRowsInTheParent(t *testing.T) {
	m := worktreeModel()
	m.sideFocus = true
	for _, it := range m.sideItems() {
		if it.key == "wt:shop/fix-tax" {
			m.sideActivateItem(it)
		}
	}
	if m.activeTab != tabSites || m.currentSite().Name != "shop" {
		t.Fatalf("expected the parent site selected, got tab %v", m.activeTab)
	}
	if m.sideFocus || m.siteTab != tabSiteOverview {
		t.Fatal("opening a worktree should focus the Overview")
	}
	rows := detailRows(m.currentSite())
	row := rows[navigableRows(rows)[m.detailCursor]]
	if row.branch != "fix-tax" {
		t.Fatalf("cursor should sit on the fix-tax rows, got %+v", row)
	}
	m.syncSideKey()
	if m.sideKey != "wt:shop/fix-tax" {
		t.Fatalf("the sidebar should keep the worktree highlighted, got %q", m.sideKey)
	}
}

func TestNewWorktreeKeyPrefillsThePaletteInTheSiteDir(t *testing.T) {
	m := worktreeModel()
	m.switchTab(tabSites)
	m.selectSiteByName("shop")
	m.focusMain()
	next, _ := m.Update(tea.KeyPressMsg{Code: 'W', Text: "W"})
	m = next.(*Model)
	if !m.paletteActive || m.paletteInput != "worktree add " || m.paletteDir != "/p/shop" {
		t.Fatalf("W should open the palette on worktree add in the site dir, got active=%v input=%q dir=%q", m.paletteActive, m.paletteInput, m.paletteDir)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(*Model)
	if m.paletteDir != "" {
		t.Fatal("closing the palette should forget the directory")
	}
}
