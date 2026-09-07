package config

import (
	"os"
	"path/filepath"
	"testing"
)

// A site whose env was rewritten to the service's domain names the container
// nowhere. Without matching the domain it drops out of every sweep that keeps
// it wired, including the one that would put the container name back when the
// domain goes away.
func TestSitesUsingService_MatchesASiteWiredToTheDomain(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Services == nil {
		cfg.Services = map[string]ServiceConfig{}
	}
	sc := cfg.Services["rustfs"]
	sc.Domain = "rustfs.test"
	cfg.Services["rustfs"] = sc
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	dir := t.TempDir()
	env := "FILESYSTEM_DISK=s3\nAWS_ENDPOINT=https://rustfs.test\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AddSite(Site{Name: "shop", Domains: []string{"shop.test"}, Path: dir}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	sites := SitesUsingService("rustfs")
	if len(sites) != 1 || sites[0].Name != "shop" {
		t.Fatalf("expected the domain-wired site to be found, got %v", sites)
	}
}

func TestSitesUsingService_StillMatchesTheContainerName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := t.TempDir()
	env := "FILESYSTEM_DISK=s3\nAWS_ENDPOINT=http://lerd-rustfs:9000\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AddSite(Site{Name: "shop", Domains: []string{"shop.test"}, Path: dir}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	if sites := SitesUsingService("rustfs"); len(sites) != 1 {
		t.Fatalf("expected the container-wired site to be found, got %v", sites)
	}
}
