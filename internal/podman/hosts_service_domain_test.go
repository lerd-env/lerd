package podman

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A presigned URL carries its host in the signature, so the app container has
// to reach the service by the same name the browser will use. Without this the
// name resolves only on the host and the app cannot sign for it at all.
func TestRenderContainerHosts_ResolvesServiceDomainsToNginx(t *testing.T) {
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
	sc := cfg.Services["rustfs"]
	sc.Domain = "rustfs.test"
	cfg.Services["rustfs"] = sc
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	reg := &config.SiteRegistry{Sites: []config.Site{{Name: "shop", Domains: []string{"shop.test"}}}}
	got := renderContainerHosts(reg, "169.254.1.2", "10.89.0.3")

	if !strings.Contains(got, "10.89.0.3 rustfs.test\n") {
		t.Errorf("service domain missing from the container hosts file:\n%s", got)
	}
	if !strings.Contains(got, "10.89.0.3 shop.test\n") {
		t.Errorf("site domain missing from the container hosts file:\n%s", got)
	}
}

func TestRenderContainerHosts_NoServiceDomainsAddsNothing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	reg := &config.SiteRegistry{Sites: []config.Site{{Name: "shop", Domains: []string{"shop.test"}}}}
	got := renderContainerHosts(reg, "169.254.1.2", "10.89.0.3")
	if strings.Count(got, "10.89.0.3") != 1 {
		t.Errorf("expected only the site entry, got:\n%s", got)
	}
}
