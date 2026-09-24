package ui

import (
	"reflect"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A dashboard in streaming mode never saw the hidden sites or the private
// workspaces, so a sidebar drag must leave them where they were.
func TestMergeWorkspaceLayoutKeepsWhatStreamingHid(t *testing.T) {
	existing := []config.Workspace{
		{Name: "Mine", Sites: []string{"shop", "secret"}},
		{Name: "Clients", Sites: []string{"client-a"}, Private: true},
	}
	sent := []WorkspaceResponse{{Name: "Mine", Sites: []string{"shop"}}}
	hidden := map[string]bool{"secret": true, "client-a": true}

	got := mergeWorkspaceLayout(sent, existing, hidden)
	want := []config.Workspace{
		{Name: "Mine", Sites: []string{"shop", "secret"}},
		{Name: "Clients", Sites: []string{"client-a"}, Private: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merge = %+v, want %+v", got, want)
	}
}

func TestMergeWorkspaceLayoutKeepsThePrivateFlagOfASentWorkspace(t *testing.T) {
	existing := []config.Workspace{{Name: "Clients", Private: true}}
	sent := []WorkspaceResponse{{Name: "Clients", Sites: []string{"client-a"}}}
	got := mergeWorkspaceLayout(sent, existing, nil)
	if len(got) != 1 || !got[0].Private {
		t.Errorf("merge dropped the private flag: %+v", got)
	}
}

func TestMergeWorkspaceLayoutStillUngroupsAVisibleSite(t *testing.T) {
	existing := []config.Workspace{{Name: "Mine", Sites: []string{"shop"}}}
	got := mergeWorkspaceLayout([]WorkspaceResponse{{Name: "Mine", Sites: []string{}}}, existing, nil)
	if len(got[0].Sites) != 0 {
		t.Errorf("visible site should be ungrouped, got %+v", got)
	}
}

func TestVisibleWorkspacesHidesPrivateOnesAndHiddenMembers(t *testing.T) {
	list := []config.Workspace{
		{Name: "Mine", Sites: []string{"shop", "secret"}},
		{Name: "Clients", Sites: []string{"client-a"}, Private: true},
	}
	got := visibleWorkspaces(list, true, map[string]bool{"secret": true, "client-a": true})
	want := []WorkspaceResponse{{Name: "Mine", Sites: []string{"shop"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("visibleWorkspaces = %+v, want %+v", got, want)
	}
	all := visibleWorkspaces(list, false, nil)
	if len(all) != 2 || !all[1].Private {
		t.Errorf("streaming off should list everything with its flag, got %+v", all)
	}
}
