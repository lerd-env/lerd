package config

import (
	"reflect"
	"testing"
)

func streamingReg() *SiteRegistry {
	return &SiteRegistry{Sites: []Site{
		{Name: "shop"},
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

func TestStreamingHiddenCoversPrivateWorkspacesAndTheirSecondaries(t *testing.T) {
	cfg := wsCfg(
		Workspace{Name: "Clients", Sites: []string{"client-a", "portal"}, Private: true},
		Workspace{Name: "Mine", Sites: []string{"shop"}},
	)
	cfg.UI.StreamingEnabled, cfg.UI.StreamingMode = true, true
	want := map[string]bool{"client-a": true, "portal": true, "portal-admin": true}
	if got := cfg.StreamingHidden(streamingReg()); !reflect.DeepEqual(got, want) {
		t.Errorf("StreamingHidden() = %v, want %v", got, want)
	}
}

func TestVisibleWorkspaceNamesDropsPrivateOnesWhileStreaming(t *testing.T) {
	cfg := wsCfg(Workspace{Name: "Clients", Private: true}, Workspace{Name: "Mine"})
	if got := cfg.VisibleWorkspaceNames(); !reflect.DeepEqual(got, []string{"Clients", "Mine"}) {
		t.Errorf("streaming off: got %v", got)
	}
	cfg.UI.StreamingEnabled, cfg.UI.StreamingMode = true, true
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

func TestNothingHidesWhileTheFeatureIsOff(t *testing.T) {
	cfg := wsCfg(Workspace{Name: "Clients", Sites: []string{"client-a"}, Private: true}, Workspace{Name: "Mine"})
	cfg.UI.StreamingMode = true
	if got := cfg.StreamingHidden(streamingReg()); len(got) != 0 {
		t.Errorf("feature off: got %v hidden, want nothing", got)
	}
	if got := cfg.VisibleWorkspaceNames(); !reflect.DeepEqual(got, []string{"Clients", "Mine"}) {
		t.Errorf("feature off: visible %v", got)
	}
}

func TestDisablingTheFeatureClearsTheMode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := SetStreamingEnabled(true); err != nil {
		t.Fatal(err)
	}
	if err := SetStreamingMode(true); err != nil {
		t.Fatal(err)
	}
	if err := SetStreamingEnabled(false); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadGlobal()
	if cfg.UI.StreamingEnabled || cfg.UI.StreamingMode {
		t.Errorf("after disabling: enabled %v mode %v, want both off", cfg.UI.StreamingEnabled, cfg.UI.StreamingMode)
	}
}
