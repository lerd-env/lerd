package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The add action only takes up a service the dashboard actually offered, so a
// crafted request cannot write an arbitrary preset into a project's .lerd.yaml.
func TestAddSiteServiceRefusesUnsuggested(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	site := &config.Site{Name: "shop", Path: dir}

	if err := addSiteService(site, "solr"); err == nil {
		t.Fatal("an unsuggested service was accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, ".lerd.yaml")); !os.IsNotExist(err) {
		t.Error(".lerd.yaml was written for a refused service")
	}
}

// Adding a suggested service is asking for it, so one removed earlier with
// `lerd service remove` comes back rather than being wired into .env while
// link keeps skipping it.
func TestAddSiteServiceBringsBackARemovedService(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("REDIS_HOST=127.0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveFramework(&config.Framework{
		Name: "testfw",
		Env: config.FrameworkEnvConf{Services: map[string]config.FrameworkServiceDef{
			"redis": {Detect: []config.FrameworkServiceDetect{{Key: "REDIS_HOST"}}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	site := &config.Site{Name: "shop", Path: dir, Framework: "testfw"}
	if err := config.SetServiceRemoved("redis", true); err != nil {
		t.Fatal(err)
	}
	var ran [][]string
	prev := runLerdFn
	runLerdFn = func(_ string, args ...string) ([]byte, error) {
		if config.ServiceIsRemoved("redis") {
			t.Errorf("lerd %v ran while redis was still marked removed", args)
		}
		ran = append(ran, args)
		return nil, nil
	}
	t.Cleanup(func() { runLerdFn = prev })

	if err := addSiteService(site, "redis"); err != nil {
		t.Fatalf("addSiteService: %v", err)
	}
	if len(ran) != 2 {
		t.Fatalf("expected link and env to run, got %v", ran)
	}
}

// A service taken off with the X stays offered, and adding it back has to lift
// the decline before env runs, or env would skip the very service just asked for.
func TestAddSiteServiceLiftsTheDeclineBeforeWiring(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("REDIS_HOST=127.0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveFramework(&config.Framework{
		Name: "testfw",
		Env: config.FrameworkEnvConf{Services: map[string]config.FrameworkServiceDef{
			"redis": {Detect: []config.FrameworkServiceDetect{{Key: "REDIS_HOST"}}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: dir, Framework: "testfw"}); err != nil {
		t.Fatal(err)
	}
	if err := config.SetSiteServiceDeclined("shop", "redis", true); err != nil {
		t.Fatal(err)
	}
	site, _ := config.FindSite("shop")
	prev := runLerdFn
	runLerdFn = func(_ string, args ...string) ([]byte, error) {
		if s, _ := config.FindSite("shop"); s.DeclinesService("redis") {
			t.Errorf("lerd %v ran while redis was still declined for the site", args)
		}
		return nil, nil
	}
	t.Cleanup(func() { runLerdFn = prev })

	if err := addSiteService(site, "redis"); err != nil {
		t.Fatalf("addSiteService: %v", err)
	}
}
