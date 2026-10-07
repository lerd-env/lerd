package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/geodro/lerd/internal/siteinfo"
)

func branchModel(t *testing.T) *Model {
	t.Helper()
	root := t.TempDir()
	wtPath := filepath.Join(root, "shop-feat")
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte("APP_URL=https://feat.shop.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := NewModel("test")
	m.width, m.height = 160, 45
	m.snap = Snapshot{
		Sites: []siteinfo.EnrichedSite{{
			Name: "shop", Domains: []string{"shop.test"}, Path: root, FPMRunning: true, PHPVersion: "8.4", Branch: "main",
			Worktrees: []siteinfo.WorktreeInfo{
				{Branch: "feat", Domain: "feat.shop.test", Path: wtPath, PHPVersion: "8.3",
					FrameworkWorkers: []siteinfo.WorkerInfo{{Name: "vite"}}},
				{Branch: "fix", Domain: "fix.shop.test", Path: filepath.Join(root, "shop-fix")},
			},
		}},
		Status: StatusRow{TLD: "test", DNSOk: true, NginxRunning: true, WatcherRunning: true},
	}
	m.switchTab(tabSites)
	m.focusMain()
	return m
}

func TestSidebarDoesNotListWorktrees(t *testing.T) {
	m := branchModel(t)
	for _, it := range m.sideItems() {
		if strings.HasPrefix(it.key, "wt:") {
			t.Fatalf("worktrees belong in the site view, found %q in the sidebar", it.key)
		}
	}
}

func TestSiteHeaderShowsABranchTabPerWorktree(t *testing.T) {
	m := branchModel(t)
	out := ansi.Strip(strings.Join(m.siteHeader(m.currentSite(), 120), "\n"))
	for _, want := range []string{" main ", " feat ", " fix "} {
		if !strings.Contains(out, want) {
			t.Fatalf("branch tabs missing %q:\n%s", want, out)
		}
	}
}

func TestSiteRowsFollowTheSelectedBranch(t *testing.T) {
	m := branchModel(t)
	for _, r := range m.siteRows(m.currentSite()) {
		if r.branch != "" {
			t.Fatalf("the main checkout should not show worktree rows, got %+v", r)
		}
	}
	m.timingScope = 1
	rows := m.siteRows(m.currentSite())
	nav := navigableRows(rows)
	if len(nav) == 0 {
		t.Fatal("the feat worktree should have controls")
	}
	for _, i := range nav {
		if rows[i].branch != "feat" {
			t.Fatalf("feat should only show its own rows, got %+v", rows[i])
		}
	}
}

func TestBranchKeyCyclesWorktreesAndScopesTheHeader(t *testing.T) {
	m := branchModel(t)
	m.detailCursor = 3
	next, _ := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = next.(*Model)
	if m.timingScope != 1 || m.detailCursor != 0 {
		t.Fatalf("b should move to the first worktree and reset the cursor, got scope %d cursor %d", m.timingScope, m.detailCursor)
	}
	out := ansi.Strip(m.renderSiteView(140, 40))
	if !strings.Contains(out, "http://feat.shop.test") || !strings.Contains(out, "php 8.3") || strings.Contains(out, "fix.shop.test") {
		t.Fatalf("the header should describe the worktree:\n%s", out)
	}
	if !strings.Contains(out, "Sites  ›  shop.test  ›  feat") {
		t.Fatalf("the breadcrumb should name the worktree:\n%s", out)
	}
}

func TestEnvTabReadsTheWorktreeEnv(t *testing.T) {
	m := branchModel(t)
	m.timingScope = 1
	out := stripANSI(strings.Join(siteEnvContentLines(m, m.currentSite(), 100), "\n"))
	if !strings.Contains(out, "feat.shop.test") {
		t.Fatalf("Env should read the worktree's .env:\n%s", out)
	}
}

func TestSwitchingSitesReturnsToTheMainCheckout(t *testing.T) {
	m := branchModel(t)
	m.snap.Sites = append(m.snap.Sites, siteinfo.EnrichedSite{Name: "blog", Domains: []string{"blog.test"}})
	m.selectSiteByName("shop")
	m.timingScope = 1
	m.sideFocus = true
	m.sideMove(-1) // shop -> blog
	if m.timingScope != 0 {
		t.Fatalf("a different site should open on its own checkout, scope %d", m.timingScope)
	}
}
