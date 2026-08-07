package sitedoctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
)

// registerSite sandboxes the registry and nginx conf.d and registers one site
// at a project directory of its own. The path is resolved the way the registry
// resolves it, so the returned site renders the same vhost the stored one does
// on a host whose temp directory is reached through a symlink.
func registerSite(t *testing.T, site config.Site) config.Site {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	site.Path = config.CanonicalPath(t.TempDir())
	if err := config.AddSite(site); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	return site
}

// The vhost decides whether a site can be served at all, and nothing rewrites
// it after the moment it was written, so the doctor has to say when it no
// longer matches what lerd would write.
func TestCheckVhost_reportsADriftedVhost(t *testing.T) {
	site := registerSite(t, config.Site{Name: "myapp", Domains: []string{"myapp.test"}, PHPVersion: "8.3"})
	if err := nginx.GenerateVhost(site, "8.3"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}

	site.PHPVersion = "8.4"
	if err := config.AddSite(site); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	c, ok := checkVhost(site.Path)
	if !ok {
		t.Fatal("no vhost check produced for a registered site")
	}
	if c.Status != StatusWarn {
		t.Errorf("status = %q, want warn", c.Status)
	}
	if c.Fix != FixVhostRegenerate {
		t.Errorf("fix = %q, want %q", c.Fix, FixVhostRegenerate)
	}
	if !strings.Contains(c.Detail, "php84") {
		t.Errorf("detail = %q, want the difference named", c.Detail)
	}
}

func TestCheckVhost_passesForACurrentVhost(t *testing.T) {
	site := registerSite(t, config.Site{Name: "myapp", Domains: []string{"myapp.test"}, PHPVersion: "8.4"})
	if err := nginx.GenerateVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}

	c, ok := checkVhost(site.Path)
	if !ok || c.Status != StatusOK {
		t.Errorf("check = %+v (ok=%v), want an ok vhost check", c, ok)
	}
}

// A directory that is not a registered site has no vhost to be wrong, and a
// worktree is served by a vhost of its own, so neither is checked.
func TestCheckVhost_skipsAnUnregisteredPath(t *testing.T) {
	registerSite(t, config.Site{Name: "myapp", Domains: []string{"myapp.test"}, PHPVersion: "8.4"})

	if c, ok := checkVhost(t.TempDir()); ok {
		t.Errorf("check = %+v, want none for an unregistered path", c)
	}
}

// A paused site's vhost is its landing page on purpose.
func TestCheckVhost_skipsAPausedSite(t *testing.T) {
	site := registerSite(t, config.Site{Name: "myapp", Domains: []string{"myapp.test"}, PHPVersion: "8.4", Paused: true})

	if c, ok := checkVhost(site.Path); ok {
		t.Errorf("check = %+v, want none for a paused site", c)
	}
}

// The fix writes the current vhost and leaves the check passing.
func TestFixVhost_regeneratesTheSitesVhost(t *testing.T) {
	site := registerSite(t, config.Site{Name: "myapp", Domains: []string{"myapp.test"}, PHPVersion: "8.3"})
	if err := nginx.GenerateVhost(site, "8.3"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	site.PHPVersion = "8.4"
	if err := config.AddSite(site); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	if err := FixVhost(site.Path); err != nil {
		t.Fatalf("FixVhost: %v", err)
	}

	conf, err := os.ReadFile(filepath.Join(config.NginxConfD(), "myapp.test.conf"))
	if err != nil {
		t.Fatalf("reading the regenerated vhost: %v", err)
	}
	if !strings.Contains(string(conf), "lerd-php84-fpm") {
		t.Errorf("vhost was not regenerated:\n%s", conf)
	}
	if c, _ := checkVhost(site.Path); c.Status != StatusOK {
		t.Errorf("check = %+v, want ok after the fix", c)
	}
}
