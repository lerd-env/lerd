package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func isolateConfig(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, ".local", "share"))
	for _, d := range []string{
		config.ConfigDir(),
		config.DataDir(),
		config.NginxConfD(),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
}

func TestRemoveStale_removesDeletedNonParkedSite(t *testing.T) {
	isolateConfig(t)

	liveDir := t.TempDir()
	deletedDir := filepath.Join(t.TempDir(), "ghost")

	reg := &config.SiteRegistry{Sites: []config.Site{
		{Name: "live", Domains: []string{"live.test"}, Path: liveDir},
		{Name: "ghost", Domains: []string{"ghost.test"}, Path: deletedDir},
	}}
	if err := config.SaveSites(reg); err != nil {
		t.Fatal(err)
	}

	if !removeStale(&config.GlobalConfig{}) {
		t.Fatal("expected removeStale to report a removal")
	}

	after, err := config.LoadSites()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(after.Sites))
	for _, s := range after.Sites {
		names = append(names, s.Name)
	}
	if len(names) != 1 || names[0] != "live" {
		t.Errorf("expected only [live] after sweep, got %v", names)
	}
}

func TestRemoveStale_keepsLiveSite(t *testing.T) {
	isolateConfig(t)

	liveDir := t.TempDir()
	reg := &config.SiteRegistry{Sites: []config.Site{
		{Name: "live", Domains: []string{"live.test"}, Path: liveDir},
	}}
	if err := config.SaveSites(reg); err != nil {
		t.Fatal(err)
	}

	if removeStale(&config.GlobalConfig{}) {
		t.Errorf("expected no removals when all site dirs exist")
	}
	after, _ := config.LoadSites()
	if len(after.Sites) != 1 {
		t.Errorf("expected live site preserved, got %d sites", len(after.Sites))
	}
}

func TestRemoveStale_skipsIgnoredSites(t *testing.T) {
	isolateConfig(t)

	// Ignored site with a deleted path should NOT be touched — the user has
	// intentionally parked it in the "ignored" state and the sweep shouldn't
	// reap it out from under them.
	reg := &config.SiteRegistry{Sites: []config.Site{
		{Name: "archived", Domains: []string{"archived.test"}, Path: "/var/empty/does-not-exist", Ignored: true},
	}}
	if err := config.SaveSites(reg); err != nil {
		t.Fatal(err)
	}

	if removeStale(&config.GlobalConfig{}) {
		t.Errorf("removeStale should not touch ignored sites")
	}
	after, _ := config.LoadSites()
	if len(after.Sites) != 1 {
		t.Errorf("ignored site should be preserved, got %d sites", len(after.Sites))
	}
}
