package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Declaring only records a service the site already reaches through its env
// file, so a crafted request cannot put an arbitrary preset in .lerd.yaml.
func TestDeclareSiteServiceRefusesAServiceTheSiteDoesNotUse(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	site := &config.Site{Name: "shop", Path: dir}

	if err := declareSiteService(site, "solr"); err == nil {
		t.Fatal("a service the site does not use was declared")
	}
	if _, err := os.Stat(filepath.Join(dir, ".lerd.yaml")); !os.IsNotExist(err) {
		t.Error(".lerd.yaml was written for a refused service")
	}
}

// Removing fails on a service .lerd.yaml does not list and leaves the env file
// alone, so a stray request cannot un-wire a service the project never declared.
func TestRemoveSiteServiceLeavesEnvWhenNotDeclared(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	env := "REDIS_HOST=lerd-redis\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	site := &config.Site{Name: "shop", Path: dir}

	if err := removeSiteService(site, "redis"); err == nil {
		t.Fatal("removing an undeclared service succeeded")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".env")); string(got) != env {
		t.Errorf(".env changed to %q", got)
	}
}

// The X takes the service off the site for good: its env keys stay behind at
// the project's example values, and without a record of the removal the next
// `lerd env` detects them and wires the service straight back.
func TestRemoveSiteServiceRecordsTheDecline(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("services:\n  - redis\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("REDIS_HOST=lerd-redis\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: dir}); err != nil {
		t.Fatal(err)
	}
	site, err := config.FindSite("shop")
	if err != nil {
		t.Fatal(err)
	}

	if err := removeSiteService(site, "redis"); err != nil {
		t.Fatalf("removeSiteService: %v", err)
	}
	after, _ := config.FindSite("shop")
	if !after.DeclinesService("redis") {
		t.Errorf("declined services = %v, want redis recorded", after.DeclinedServices)
	}
}
