package config

import (
	"reflect"
	"testing"
)

func streamingReg() *SiteRegistry {
	return &SiteRegistry{Sites: []Site{
		{Name: "shop"},
		{Name: "secret", Private: true},
		{Name: "client-a"},
		{Name: "portal", Group: "portal"},
		{Name: "portal-admin", Group: "portal", GroupSubdomain: "admin"},
	}}
}

func TestStreamingHiddenIsEmptyWhenStreamingIsOff(t *testing.T) {
	cfg := wsCfg(Workspace{Name: "Clients", Sites: []string{"client-a"}, Private: true})
	if got := cfg.StreamingHidden(streamingReg()); len(got) != 0 {
		t.Errorf("streaming off: got %v, want nothing hidden", got)
	}
}

func TestStreamingHiddenCoversPrivateSitesAndWorkspaces(t *testing.T) {
	cfg := wsCfg(
		Workspace{Name: "Clients", Sites: []string{"client-a", "portal"}, Private: true},
		Workspace{Name: "Mine", Sites: []string{"shop"}},
	)
	cfg.UI.StreamingMode = true
	want := map[string]bool{"secret": true, "client-a": true, "portal": true, "portal-admin": true}
	if got := cfg.StreamingHidden(streamingReg()); !reflect.DeepEqual(got, want) {
		t.Errorf("StreamingHidden() = %v, want %v", got, want)
	}
}

func TestStreamingHiddenFollowsAPrivateGroupMain(t *testing.T) {
	reg := streamingReg()
	reg.Sites[3].Private = true
	cfg := wsCfg()
	cfg.UI.StreamingMode = true
	got := cfg.StreamingHidden(reg)
	if !got["portal-admin"] {
		t.Errorf("secondary of a private main should hide, got %v", got)
	}
}

func TestVisibleWorkspaceNamesDropsPrivateOnesWhileStreaming(t *testing.T) {
	cfg := wsCfg(Workspace{Name: "Clients", Private: true}, Workspace{Name: "Mine"})
	if got := cfg.VisibleWorkspaceNames(); !reflect.DeepEqual(got, []string{"Clients", "Mine"}) {
		t.Errorf("streaming off: got %v", got)
	}
	cfg.UI.StreamingMode = true
	if got := cfg.VisibleWorkspaceNames(); !reflect.DeepEqual(got, []string{"Mine"}) {
		t.Errorf("streaming on: got %v", got)
	}
}

func TestSetWorkspaceLayoutKeepsThePrivateFlag(t *testing.T) {
	cfg := wsCfg(Workspace{Name: "Clients", Private: true})
	if err := cfg.setWorkspaceLayout([]Workspace{{Name: "Clients", Private: true}, {Name: "New"}}); err != nil {
		t.Fatal(err)
	}
	if !cfg.Workspaces[0].Private || cfg.Workspaces[1].Private {
		t.Errorf("layout lost the private flag: %+v", cfg.Workspaces)
	}
}

func TestSiteYAMLRoundTripsPrivate(t *testing.T) {
	if !(Site{Name: "x", Private: true}).toYAML().toSite().Private {
		t.Error("Private did not survive the YAML round trip")
	}
}
