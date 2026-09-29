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
