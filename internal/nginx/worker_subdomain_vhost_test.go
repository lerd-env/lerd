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

// A worktree served on the label before the worker claimed it loses its vhost,
// or nginx would keep answering the shared server_name from the worktree file.
func TestGenerateVhost_workerSubdomainTakesOverAWorktreeVhost(t *testing.T) {
	confD := setupConfD(t)
	site := proxySite(t, "app", &config.WorkerProxy{Subdomain: "docs", DefaultPort: 5180})
	if err := os.MkdirAll(confD, 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"docs.app.test.conf", "docs.app.test-ssl.conf"} {
		if err := os.WriteFile(filepath.Join(confD, f), []byte("server {}"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"docs.app.test.conf", "docs.app.test-ssl.conf"} {
		if _, err := os.Stat(filepath.Join(confD, f)); !os.IsNotExist(err) {
			t.Errorf("worktree vhost %s survived the worker claiming its host (err=%v)", f, err)
		}
	}
	readConf(t, filepath.Join(confD, "app.test.worker-docs.conf"))
}

// Two workers claiming one label: the first keeps it, the second is refused
// rather than silently overwriting the route.
func TestGenerateVhost_duplicateWorkerSubdomainKeepsTheFirst(t *testing.T) {
	confD := setupConfD(t)
	site := proxySite(t, "app", nil)
	proj, err := config.LoadProjectConfig(site.Path)
	if err != nil {
		t.Fatal(err)
	}
	proj.CustomWorkers = map[string]config.FrameworkWorker{
		"alpha": {Command: "a", Proxy: &config.WorkerProxy{Subdomain: "docs", DefaultPort: 5101}},
		"beta":  {Command: "b", Proxy: &config.WorkerProxy{Subdomain: "docs", DefaultPort: 5102}},
	}
	if err := config.SaveProjectConfig(site.Path, proj); err != nil {
		t.Fatal(err)
	}
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatal(err)
	}
	if conf := readConf(t, filepath.Join(confD, "app.test.worker-docs.conf")); !strings.Contains(conf, ":5101;") {
		t.Errorf("the first worker lost its subdomain:\n%s", conf)
	}
}

// A missing worker vhost, one on an old port and one for a label no longer
// declared are all drift, and regenerating repairs each.
func TestVhostDrift_coversWorkerSubdomainVhosts(t *testing.T) {
	confD := setupConfD(t)
	site := proxySite(t, "app", &config.WorkerProxy{Subdomain: "docs", DefaultPort: 5180})
	site.PHPVersion = "8.4"
	worker := filepath.Join(confD, "app.test.worker-docs.conf")
	stale := filepath.Join(confD, "app.test.worker-old.conf")
	breakIt := map[string]func(){
		"missing": func() { _ = os.Remove(worker) },
		"old port": func() {
			b, _ := os.ReadFile(worker)
			_ = os.WriteFile(worker, []byte(strings.ReplaceAll(string(b), "5180", "5999")), 0644)
		},
		"stale": func() { _ = os.WriteFile(stale, []byte("server {}"), 0644) },
	}
	for name, breakFn := range breakIt {
		if err := GenerateVhost(site, "8.4"); err != nil {
			t.Fatal(err)
		}
		if d, err := VhostDrift(site); err != nil || d.Drifted {
			t.Fatalf("%s: fresh vhosts read as drifted: %+v %v", name, d, err)
		}
		breakFn()
		if d, err := VhostDrift(site); err != nil || !d.Drifted {
			t.Errorf("%s: drift = %+v %v, want drifted", name, d, err)
		}
		if err := RegenerateVhost(site); err != nil {
			t.Fatal(err)
		}
		if d, _ := VhostDrift(site); d.Drifted {
			t.Errorf("%s: still drifted after regenerating: %s", name, d.Detail)
		}
	}
}

// Every path that rebuilds worktree vhosts, the watcher's sweep and a pause
// among them, must leave a worker's host alone, or the worktree answers on it.
func TestGenerateWorktreeVhost_leavesAWorkerSubdomainHostAlone(t *testing.T) {
	confD := setupConfD(t)
	site := proxySite(t, "app", &config.WorkerProxy{Subdomain: "api", DefaultPort: 5180})
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	if err := GenerateWorktreeVhost("api.app.test", wt, "8.4", "app", "api"); err != nil {
		t.Fatal(err)
	}
	if err := GeneratePausedWorktreeVhost("api.app.test", "app.test", t.TempDir(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(confD, "api.app.test.conf")); !os.IsNotExist(err) {
		t.Errorf("a worktree vhost landed on the worker's host (err=%v)", err)
	}
	if err := GenerateWorktreeVhost("feat.app.test", wt, "8.4", "app", "feat"); err != nil {
		t.Fatal(err)
	}
	readConf(t, filepath.Join(confD, "feat.app.test.conf"))
}

// Moving off FPM takes the worker vhosts with it: they name an upstream the site
// no longer has, and the doctor reads one left behind as drift.
func TestLeavingFPMDropsWorkerSubdomainVhosts(t *testing.T) {
	confD := setupConfD(t)
	site := proxySite(t, "app", &config.WorkerProxy{Subdomain: "docs", DefaultPort: 5180})
	worker := filepath.Join(confD, "app.test.worker-docs.conf")
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatal(err)
	}
	site.Runtime = "frankenphp"
	if err := GenerateFrankenPHPVhost(site); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(worker); !os.IsNotExist(err) {
		t.Errorf("worker vhost survived the switch to FrankenPHP (err=%v)", err)
	}
	if err := os.WriteFile(worker, []byte("server {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if d, err := VhostDrift(site); err != nil || !d.Drifted {
		t.Errorf("drift = %+v %v, want the leftover reported", d, err)
	}
	if err := RegenerateVhost(site); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(worker); !os.IsNotExist(err) {
		t.Errorf("regenerating kept the leftover worker vhost (err=%v)", err)
	}
}
