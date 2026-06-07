package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
)

// A host-proxy site has no FPM container, so workers must not get a bogus
// lerd-php-fpm dependency: resolveWorkerFPMUnit returns "" so the host worker
// writer skips the FPM ordering block entirely.
func TestResolveWorkerFPMUnit_hostProxyReturnsEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	reg := &config.SiteRegistry{Sites: []config.Site{
		{Name: "proxysite", Domains: []string{"proxysite.test"}, Path: t.TempDir(), HostPort: 5173, HostCommand: "npm run dev"},
		{Name: "phpsite", Domains: []string{"phpsite.test"}, Path: t.TempDir()},
	}}
	if err := config.SaveSites(reg); err != nil {
		t.Fatalf("SaveSites: %v", err)
	}

	if got := resolveWorkerFPMUnit("proxysite", ""); got != "" {
		t.Errorf("host-proxy site FPM unit = %q, want empty", got)
	}
	if got := resolveWorkerFPMUnit("phpsite", "8.4"); got != "lerd-php84-fpm" {
		t.Errorf("php site FPM unit = %q, want lerd-php84-fpm", got)
	}
}

// reservedHostPorts must include the ports lerd services publish even when the
// container is stopped, so a host-proxy dev server is never assigned a port a
// service (e.g. gotenberg on 3000) will reclaim when it starts.
func TestReservedHostPorts_includesServicePorts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	reserved := reservedHostPorts("")
	if !reserved[3000] {
		t.Errorf("expected gotenberg's published port 3000 to be reserved, got %v", reserved)
	}
}

// Reserving service ports must not break the exceptSite contract: re-running
// init on a host-proxy site already sitting on a service-coinciding port (e.g.
// 3000) keeps that port instead of silently bumping it.
func TestReservedHostPorts_exceptSiteKeepsServiceCoincidingPort(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	reg := &config.SiteRegistry{Sites: []config.Site{
		{Name: "node-app", Domains: []string{"node-app.test"}, Path: t.TempDir(), HostPort: 3000, HostCommand: "npm run dev"},
	}}
	if err := config.SaveSites(reg); err != nil {
		t.Fatalf("SaveSites: %v", err)
	}

	if reservedHostPorts("node-app")[3000] {
		t.Error("exceptSite's own port 3000 must not be reserved against itself, even though gotenberg also publishes 3000")
	}
	if !reservedHostPorts("")[3000] {
		t.Error("with no exceptSite, 3000 should still be reserved (gotenberg)")
	}
}

// The High fix: securing a host-proxy site's worktree must render a reverse
// proxy vhost (proxy_pass) pointing at the dev-server port, not a PHP fastcgi
// vhost. The wired certs hook is what makes this happen.
func TestRegenerateHostProxyWorktreeVhost_writesProxyNotFastcgi(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if certs.RegenerateHostProxyWorktreeVhost == nil {
		t.Fatal("cli init must wire certs.RegenerateHostProxyWorktreeVhost")
	}

	sitePath := t.TempDir()
	site := config.Site{Name: "astro", Domains: []string{"astro.test"}, Path: sitePath, HostPort: 4321, HostCommand: "npm run dev"}

	wtPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(wtPath, ".env"), []byte("PORT=4322\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wtDomain := "feature.astro.test"

	if err := certs.RegenerateHostProxyWorktreeVhost(site, wtPath, wtDomain, false); err != nil {
		t.Fatalf("hook: %v", err)
	}

	conf := filepath.Join(config.NginxConfD(), wtDomain+".conf")
	data, err := os.ReadFile(conf)
	if err != nil {
		t.Fatalf("read worktree vhost: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "proxy_pass") {
		t.Errorf("host-proxy worktree vhost must reverse-proxy; got:\n%s", s)
	}
	if strings.Contains(s, "fastcgi_pass") {
		t.Errorf("host-proxy worktree vhost must NOT be a PHP fastcgi vhost; got:\n%s", s)
	}
	if !strings.Contains(s, "4322") {
		t.Errorf("host-proxy worktree vhost must point at the worktree dev-server port 4322; got:\n%s", s)
	}
}
