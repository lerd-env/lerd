package siteops

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// recordDevServerRefresh captures the sites handed to the dev server refresh, so
// a test can assert that a path which moves a site's addresses reaches it.
func recordDevServerRefresh(t *testing.T) *[]string {
	t.Helper()
	var seen []string
	orig := RefreshDevServers
	RefreshDevServers = func(site *config.Site) {
		if site != nil {
			seen = append(seen, site.Name)
		}
	}
	t.Cleanup(func() { RefreshDevServers = orig })
	return &seen
}

// A dev server reads the site's scheme once, at start, so unsecuring a site has
// to reach it or every asset URL on the page keeps naming https.
func TestSetSecuredRefreshesTheDevServer(t *testing.T) {
	stubSecureDeps(t)
	projectDir := withTempEnv(t)
	seen := recordDevServerRefresh(t)

	site := &config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: projectDir, Secured: true}
	if err := config.AddSite(*site); err != nil {
		t.Fatalf("seed site: %v", err)
	}

	if err := SetSecured(site, false); err != nil {
		t.Fatalf("SetSecured: %v", err)
	}

	if len(*seen) != 1 || (*seen)[0] != "myapp" {
		t.Errorf("dev server refreshed for %v, want [myapp]", *seen)
	}
}

// Every caller that changes a site's domains regenerates its vhost, so that is
// where the dev server's allowed hosts are brought along.
func TestRegenerateSiteVhostRefreshesTheDevServer(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	seen := recordDevServerRefresh(t)

	site := &config.Site{
		Name:       "myapp",
		Domains:    []string{"myapp.test", "alias.test"},
		Path:       t.TempDir(),
		PHPVersion: "8.4",
	}
	if err := RegenerateSiteVhost(site, "myapp.test"); err != nil {
		t.Fatalf("RegenerateSiteVhost: %v", err)
	}

	if len(*seen) != 1 || (*seen)[0] != "myapp" {
		t.Errorf("dev server refreshed for %v, want [myapp]", *seen)
	}
}
