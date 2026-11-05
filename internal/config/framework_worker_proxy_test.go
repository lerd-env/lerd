package config

import "testing"

// A site can run an asset server next to a websocket server, so every worker
// that declares a proxy gets one, in a stable order.
func TestDetectProxies_EveryWorkerNotJustTheFirst(t *testing.T) {
	fw := &Framework{Workers: map[string]FrameworkWorker{
		"reverb": {Proxy: &WorkerProxy{Paths: []string{"/app"}, PortEnvKey: "REVERB_SERVER_PORT"}},
		"vite":   {Proxy: &WorkerProxy{Paths: []string{"/build"}, Upstream: "host", Port: "pinned"}},
		"queue":  {Command: "php artisan queue:work"},
	}}

	found := fw.DetectProxies(t.TempDir())
	if len(found) != 2 {
		t.Fatalf("proxies = %d, want 2", len(found))
	}
	if found[0].Worker != "reverb" || found[1].Worker != "vite" {
		t.Errorf("order = %q, %q, want reverb then vite", found[0].Worker, found[1].Worker)
	}
	if !found[1].Proxy.OnHost() || !found[1].Proxy.PinnedPort() {
		t.Errorf("vite proxy = %+v, want a pinned host proxy", found[1].Proxy)
	}
	if found[0].Proxy.OnHost() || found[0].Proxy.PinnedPort() {
		t.Errorf("reverb proxy = %+v, want the container default and an env port", found[0].Proxy)
	}
}

// A worker subdomain lands in a host name, so it takes the labels a worktree
// branch sanitises to and refuses anything else by name.
func TestValidateSubdomainLabel(t *testing.T) {
	for _, ok := range []string{"docs", "api-v2", "a1"} {
		if err := ValidateSubdomainLabel(ok); err != nil {
			t.Errorf("ValidateSubdomainLabel(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "Docs", "docs.site", "-docs", "docs-", "a--b", "docs_x", "a b"} {
		if err := ValidateSubdomainLabel(bad); err == nil {
			t.Errorf("ValidateSubdomainLabel(%q) = nil, want an error", bad)
		}
	}
}

// A worker's subdomain host belongs to its site, so traffic to it wakes and
// keeps that site awake; a host no worker declares stays unresolved.
func TestFindSiteByHost_resolvesWorkerSubdomain(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	proj := &ProjectConfig{Framework: "laravel", CustomWorkers: map[string]FrameworkWorker{
		"docs": {Command: "npx vitepress dev", Proxy: &WorkerProxy{Subdomain: "docs"}},
	}}
	if err := SaveProjectConfig(dir, proj); err != nil {
		t.Fatal(err)
	}
	if err := AddSite(Site{Name: "app", Domains: []string{"app.test"}, Path: dir, Framework: "laravel"}); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"app.test", "docs.app.test"} {
		if s, err := FindSiteByHost(host); err != nil || s.Name != "app" {
			t.Errorf("FindSiteByHost(%q) = %v, %v, want site app", host, s, err)
		}
	}
	if s, err := FindSiteByHost("other.app.test"); err == nil {
		t.Errorf("FindSiteByHost(other.app.test) = %s, want not found", s.Name)
	}
}
