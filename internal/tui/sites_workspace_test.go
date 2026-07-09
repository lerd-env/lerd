package tui

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteinfo"
)

func workspacesFixture() []config.Workspace {
	return []config.Workspace{
		{Name: "Client Work", Sites: []string{"gamma"}},
		{Name: "Side Projects", Sites: []string{"alpha"}},
	}
}

func TestSiteSortWorkspace_OrdersByConfigOrderThenName(t *testing.T) {
	got := filteredSortedSites(sitesFixture(), "", siteSortWorkspace, workspacesFixture())
	want := []string{"gamma", "alpha", "beta"} // Client Work, Side Projects, then ungrouped
	if strings.Join(names(got), ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", names(got), want)
	}
}

func TestSiteSortWorkspace_UngroupedSitesTrailInNameOrder(t *testing.T) {
	got := filteredSortedSites(sitesFixture(), "", siteSortWorkspace, nil)
	want := []string{"alpha", "beta", "gamma"}
	if strings.Join(names(got), ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", names(got), want)
	}
}

func TestSiteSortWorkspace_LabelIsWorkspace(t *testing.T) {
	if got := siteSortWorkspace.label(); got != "workspace" {
		t.Errorf("label = %q, want workspace", got)
	}
}

// A group secondary follows its main, so a group is never split across two
// workspace sections even though only the main is named in the config.
func TestSiteWorkspaces_SecondaryFollowsItsMain(t *testing.T) {
	list := []siteinfo.EnrichedSite{
		{Name: "astrolov", Group: "astrolov"},
		{Name: "admin", Group: "astrolov", GroupSubdomain: "admin"},
		{Name: "orphan", Group: "gone", GroupSubdomain: "admin"},
		{Name: "solo"},
	}
	workspaces := []config.Workspace{{Name: "Client Work", Sites: []string{"astrolov"}}, {Name: "Other", Sites: []string{"orphan"}}}

	of := siteWorkspaces(list, workspaces)
	if of["admin"] != "Client Work" {
		t.Errorf("secondary workspace = %q, want Client Work", of["admin"])
	}
	if of["orphan"] != "Other" {
		t.Errorf("a secondary with no main falls back to its own: got %q", of["orphan"])
	}
	if of["solo"] != "" {
		t.Errorf("solo workspace = %q, want ungrouped", of["solo"])
	}
}

func TestWorkspaceRanks_UngroupedRanksLast(t *testing.T) {
	rank := workspaceRanks(workspacesFixture())
	if rank["Client Work"] != 0 || rank["Side Projects"] != 1 {
		t.Errorf("ranks = %v", rank)
	}
	if rank[""] != 2 {
		t.Errorf("ungrouped rank = %d, want 2", rank[""])
	}
}

func TestRenderGroupedSiteRows_InsertsHeadersAndKeepsCursorOnItsSite(t *testing.T) {
	sites := filteredSortedSites(sitesFixture(), "", siteSortWorkspace, workspacesFixture())

	rows, cursorLine := renderGroupedSiteRows(sites, workspacesFixture(), 1, true, 40)
	joined := strings.Join(rows, "\n")
	for _, want := range []string{"Client Work", "Side Projects", "gamma", "alpha", "beta"} {
		if !strings.Contains(joined, want) {
			t.Errorf("rows missing %q:\n%s", want, joined)
		}
	}
	// cursor 1 is alpha, which sits under the Side Projects header.
	if !strings.Contains(rows[cursorLine], "alpha") {
		t.Errorf("cursorLine %d = %q, want the alpha row", cursorLine, rows[cursorLine])
	}
}

// Headers shift the rendered lines, so the cursor line must not equal the flat
// index; otherwise the viewport scrolls to the wrong row.
func TestRenderGroupedSiteRows_CursorLineAccountsForHeaders(t *testing.T) {
	sites := filteredSortedSites(sitesFixture(), "", siteSortWorkspace, workspacesFixture())
	_, cursorLine := renderGroupedSiteRows(sites, workspacesFixture(), 0, true, 40)
	if cursorLine == 0 {
		t.Error("cursorLine ignored the leading workspace header")
	}
}

// Ungrouped sites get no header of their own, matching the sidebar.
func TestRenderGroupedSiteRows_NoHeaderForUngrouped(t *testing.T) {
	sites := filteredSortedSites(sitesFixture(), "", siteSortWorkspace, workspacesFixture())
	rows, _ := renderGroupedSiteRows(sites, workspacesFixture(), 0, true, 40)
	joined := strings.Join(rows, "\n")
	for _, unwanted := range []string{"Ungrouped", "ungrouped", "Other"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("rows contain an %q header:\n%s", unwanted, joined)
		}
	}
}

func TestRenderGroupedSiteRows_NoWorkspacesRendersNoHeaders(t *testing.T) {
	sites := filteredSortedSites(sitesFixture(), "", siteSortWorkspace, nil)
	rows, cursorLine := renderGroupedSiteRows(sites, nil, 0, true, 40)
	if len(rows) != len(sites) {
		t.Errorf("rows = %d, want %d (one per site, no headers)", len(rows), len(sites))
	}
	if cursorLine != 0 {
		t.Errorf("cursorLine = %d, want 0", cursorLine)
	}
}

// The whole pane, through the same path the running TUI takes.
func TestRenderSitesPane_WorkspaceModeShowsSections(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.snap.Sites = sitesFixture()
	m.snap.Workspaces = workspacesFixture()
	m.activeTab = tabSites
	m.focus = paneSites
	m.siteSort = siteSortWorkspace

	out := m.renderSites(70, 20)
	for _, want := range []string{"sort: workspace", "Client Work", "Side Projects", "gamma", "alpha", "beta"} {
		if !strings.Contains(out, want) {
			t.Errorf("pane missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Ungrouped") {
		t.Errorf("pane labelled the ungrouped sites:\n%s", out)
	}
}

// "o" cycles through every mode and lands back on name.
func TestSiteSortCycleReachesWorkspaceAndWrapsAround(t *testing.T) {
	want := []siteSortMode{siteSortStatus, siteSortFramework, siteSortWorkspace, siteSortName}
	mode := siteSortName
	for i, expect := range want {
		mode = (mode + 1) % siteSortModes
		if mode != expect {
			t.Fatalf("press %d: mode = %q, want %q", i+1, mode.label(), expect.label())
		}
	}
}
