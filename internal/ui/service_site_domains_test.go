package ui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A site that reaches a service through its domain counts as a user, so the
// list of sites shown next to that count has to name it too.
func TestBuildServiceResponse_siteDomainsAgreeWithSiteCount(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Services == nil {
		cfg.Services = map[string]config.ServiceConfig{}
	}
	cfg.Services["rustfs"] = config.ServiceConfig{Enabled: true, Domain: "rustfs.test"}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	dir := t.TempDir()
	env := "FILESYSTEM_DISK=s3\nAWS_ENDPOINT=https://rustfs.test\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: dir}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	got := buildServiceResponse("rustfs")
	if got.SiteCount != 1 || !slices.Equal(got.SiteDomains, []string{"shop.test"}) {
		t.Errorf("SiteCount %d, SiteDomains %v; want 1 and [shop.test]", got.SiteCount, got.SiteDomains)
	}
}
