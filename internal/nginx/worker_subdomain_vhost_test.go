package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A worker with only a subdomain is served on <label>.<primary>, the whole host
// proxied to its port with WebSocket upgrades, and adds nothing to the site's
// own vhost.
func TestGenerateVhost_workerSubdomainGetsItsOwnVhost(t *testing.T) {
	confD := setupConfD(t)
	site := proxySite(t, "app", &config.WorkerProxy{Subdomain: "docs", DefaultPort: 5180})
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	if main := readConf(t, filepath.Join(confD, "app.test.conf")); strings.Contains(main, "$proxybackend") {
		t.Errorf("a subdomain-only proxy added a location to the main vhost:\n%s", main)
	}
	conf := readConf(t, filepath.Join(confD, "app.test.worker-docs.conf"))
	for _, want := range []string{
		"server_name docs.app.test;",
		`set $backend "lerd-php84-fpm";`,
		"proxy_pass http://$backend:5180;",
		"proxy_set_header Upgrade $http_upgrade;",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("expected %q in:\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "443") {
		t.Errorf("plain HTTP site got a TLS server:\n%s", conf)
	}
}

// Secured, the subdomain is served under the site's own certificate, which
// already carries *.<primary>.
func TestGenerateSSLVhost_workerSubdomainUsesTheSiteCert(t *testing.T) {
	confD := setupConfD(t)
	site := proxySite(t, "app", &config.WorkerProxy{Subdomain: "docs", Upstream: "host", Port: "pinned", DefaultPort: 5180})
	site.WorkerPorts = map[string]int{"ws": 5191}
	if err := GenerateSSLVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateSSLVhost: %v", err)
	}
	conf := readConf(t, filepath.Join(confD, "app.test.worker-docs.conf"))
	for _, want := range []string{
		"listen 443 ssl;",
		"ssl_certificate /etc/nginx/certs/app.test.crt;",
		`set $backend "` + hostProxyUpstream() + `";`,
		"proxy_pass http://$backend:5191;",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("expected %q in:\n%s", want, conf)
		}
	}
}

// Path and subdomain are independent: both apply.
func TestGenerateVhost_workerPathAndSubdomainBothApply(t *testing.T) {
	confD := setupConfD(t)
	site := proxySite(t, "app", &config.WorkerProxy{Path: "/app", Subdomain: "ws", DefaultPort: 6001})
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	if main := readConf(t, filepath.Join(confD, "app.test.conf")); !strings.Contains(main, `location ~ ^/app(/|$) {`) {
		t.Errorf("path location missing:\n%s", main)
	}
	readConf(t, filepath.Join(confD, "app.test.worker-ws.conf"))
}

// An invalid label is refused rather than reshaped into some other host.
func TestGenerateVhost_invalidWorkerSubdomainIsRefused(t *testing.T) {
	confD := setupConfD(t)
	site := proxySite(t, "app", &config.WorkerProxy{Subdomain: "Docs.x", DefaultPort: 5180})
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	if m, _ := filepath.Glob(filepath.Join(confD, "app.test.worker-*.conf")); len(m) != 0 {
		t.Errorf("invalid label still wrote %v", m)
	}
}

// A subdomain a registered site owns (a group secondary) stays with that site.
func TestGenerateVhost_workerSubdomainYieldsToARegisteredSite(t *testing.T) {
	confD := setupConfD(t)
	site := proxySite(t, "app", &config.WorkerProxy{Subdomain: "admin", DefaultPort: 5180})
	if err := config.AddSite(config.Site{Name: "admin", Domains: []string{"admin.app.test"}, Path: t.TempDir(), Group: "app", GroupSubdomain: "admin"}); err != nil {
		t.Fatal(err)
	}
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(confD, "app.test.worker-admin.conf")); !os.IsNotExist(err) {
		t.Errorf("worker vhost written for a host the secondary owns (err=%v)", err)
	}
}

// A subdomain no longer declared loses its vhost on the next render, and
// removing the site's vhost (unlink, domain change) takes them all.
func TestWorkerSubdomainVhosts_staleAndRemoved(t *testing.T) {
	confD := setupConfD(t)
	site := proxySite(t, "app", &config.WorkerProxy{Subdomain: "docs", DefaultPort: 5180})
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(confD, "app.test.worker-old.conf")
	if err := os.WriteFile(stale, []byte("server {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale worker vhost survived a render (err=%v)", err)
	}
	if err := RemoveVhost("app.test"); err != nil {
		t.Fatal(err)
	}
	if m, _ := filepath.Glob(filepath.Join(confD, "app.test.worker-*.conf")); len(m) != 0 {
		t.Errorf("RemoveVhost left %v", m)
	}
}

// A paused site answers its subdomains from the landing page on its wildcard,
// so the worker vhost steps aside rather than proxying to a stopped worker.
func TestGeneratePausedVhost_dropsWorkerSubdomainVhosts(t *testing.T) {
	confD := setupConfD(t)
	site := proxySite(t, "app", &config.WorkerProxy{Subdomain: "docs", DefaultPort: 5180})
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatal(err)
	}
	if err := GeneratePausedVhost(site); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(confD, "app.test.worker-docs.conf")); !os.IsNotExist(err) {
		t.Errorf("paused site kept its worker vhost (err=%v)", err)
	}
}
