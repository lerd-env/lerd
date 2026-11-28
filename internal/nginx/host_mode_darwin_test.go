package nginx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/hostbin"
)

// hostModeLegs sets every leg of host mode to pass, for a test to break one.
func hostModeLegs(t *testing.T, sites ...config.Site) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.PHP.Runtime = config.PHPRuntimeNative
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	reg := &config.SiteRegistry{Sites: sites}
	if err := config.SaveSites(reg); err != nil {
		t.Fatal(err)
	}
	prevBin, prevFwd := hostNginxBinary, portsForwarded
	t.Cleanup(func() { hostNginxBinary, portsForwarded = prevBin, prevFwd })
	hostNginxBinary = func() (string, bool) { return "/opt/homebrew/bin/nginx", true }
	portsForwarded = func() bool { return true }
}

func TestHostMode_AppliesWhenEveryLegHolds(t *testing.T) {
	hostModeLegs(t, config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: "/srv/shop"})
	if why := HostModeBlocker(); why != "" {
		t.Fatalf("host mode blocked: %s", why)
	}
	if !OnHost() {
		t.Error("OnHost() = false with every leg holding")
	}
}

// Each leg that fails keeps nginx in its container and says which one, so
// doctor and the switch can name it rather than half-move the stack.
func TestHostMode_NamesTheLegThatFails(t *testing.T) {
	cases := []struct {
		name   string
		break_ func(t *testing.T)
		want   string
	}{
		{"kept in its container", func(t *testing.T) {
			cfg, _ := config.LoadGlobal()
			cfg.Nginx.Runtime = config.NginxRuntimeContainer
			_ = config.SaveGlobal(cfg)
		}, "not set to run on the host"},
		{"no nginx on the host", func(t *testing.T) {
			hostNginxBinary = func() (string, bool) { return "", false }
		}, "brew install nginx"},
		{"ports not forwarded", func(t *testing.T) {
			portsForwarded = func() bool { return false }
		}, "lerd install"},
		{"a site needs container nginx", func(t *testing.T) {
			reg, _ := config.LoadSites()
			reg.Sites = append(reg.Sites, config.Site{Name: "franken", Domains: []string{"franken.test"}, Path: "/srv/franken", Runtime: "frankenphp"})
			_ = config.SaveSites(reg)
		}, "franken"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hostModeLegs(t, config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: "/srv/shop"})
			c.break_(t)
			why := HostModeBlocker()
			if !strings.Contains(why, c.want) {
				t.Errorf("HostModeBlocker() = %q, want it to mention %q", why, c.want)
			}
			if OnHost() {
				t.Error("OnHost() = true with a leg broken")
			}
		})
	}
}

// The container image carries mime.types and fastcgi_params; the host's nginx
// reads lerd's own copies beside nginx.conf, the port fastcgi_params passes is
// the public one, and nginx's own redirects carry no port at all.
func TestEnsureNginxConfig_onHostWritesWhatTheImageCarried(t *testing.T) {
	hostModeLegs(t)
	if err := EnsureNginxConfig(); err != nil {
		t.Fatalf("EnsureNginxConfig: %v", err)
	}
	dir := config.NginxDir()
	conf := readConf(t, filepath.Join(dir, "nginx.conf"))
	for _, want := range []string{"include " + filepath.Join(dir, "mime.types") + ";", "port_in_redirect off;"} {
		if !strings.Contains(conf, want) {
			t.Errorf("nginx.conf missing %q:\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "user root;") {
		t.Errorf("nginx.conf still sets user root:\n%s", conf)
	}
	if !strings.Contains(readConf(t, filepath.Join(dir, "mime.types")), "text/html") {
		t.Error("mime.types was not written")
	}
	if !strings.Contains(readConf(t, filepath.Join(dir, "fastcgi_params")), "SERVER_PORT        $lerd_public_port;") {
		t.Error("fastcgi_params should pass the public port")
	}
	if !strings.Contains(readConf(t, filepath.Join(config.NginxConfD(), "_forwarded.conf")), "map ${server_port} $lerd_public_port") {
		t.Error("_forwarded.conf should define $lerd_public_port")
	}
}

// What lerd writes for the host's nginx has to load in a real one: a mapping
// rule that leaves one container path or port behind only shows up here.
func TestHostConfig_loadsInTheHostsNginx(t *testing.T) {
	bin, ok := hostbin.Look("nginx")
	if !ok {
		t.Skip("no nginx on this host")
	}
	hostModeLegs(t, config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir(), PHPVersion: "8.4"})
	if err := os.MkdirAll(config.RunDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureNginxConfig(); err != nil {
		t.Fatalf("EnsureNginxConfig: %v", err)
	}
	if err := EnsureDefaultVhost(); err != nil {
		t.Fatalf("EnsureDefaultVhost: %v", err)
	}
	if err := GenerateVhost(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir(), PHPVersion: "8.4"}, "8.4"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	if err := EnsureLerdVhost(); err != nil {
		t.Fatalf("EnsureLerdVhost: %v", err)
	}
	if err := EnsureProfilerVhost(); err != nil {
		t.Fatalf("EnsureProfilerVhost: %v", err)
	}
	out, err := exec.Command(bin, "-t", "-c", filepath.Join(config.NginxDir(), "nginx.conf"), "-p", config.NginxDir()).CombinedOutput()
	if err != nil {
		t.Fatalf("nginx -t rejected the host config: %v\n%s", err, out)
	}
}

// launchd supervises the host's nginx, so it must stay in the foreground, and
// lerd's config test runs the host binary against lerd's own config.
func TestHostNginx_RunsInTheForegroundAndTestsLerdsConfig(t *testing.T) {
	if _, ok := hostbin.Look("nginx"); !ok {
		t.Skip("no nginx on this host")
	}
	hostModeLegs(t)
	if err := os.MkdirAll(config.RunDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureNginxConfig(); err != nil {
		t.Fatalf("EnsureNginxConfig: %v", err)
	}
	if conf := readConf(t, filepath.Join(config.NginxDir(), "nginx.conf")); !strings.Contains(conf, "daemon off;") {
		t.Errorf("host nginx.conf should keep nginx in the foreground:\n%s", conf)
	}
	if out, err := Test(); err != nil || !strings.Contains(out, "test is successful") {
		t.Errorf("Test() on host = %q, %v; want the host binary's success line", out, err)
	}
	if !strings.Contains(HostServiceUnit(), "-c "+filepath.Join(config.NginxDir(), "nginx.conf")+" -p "+config.NginxDir()) {
		t.Errorf("unit should start nginx on lerd's config:\n%s", HostServiceUnit())
	}
}

// Whether the host's nginx runs is read from its pid file, never guessed: a
// stale file naming a dead process is a stopped nginx.
func TestHostNginx_RunningFollowsThePidFile(t *testing.T) {
	hostModeLegs(t)
	if err := os.MkdirAll(config.RunDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(config.RunDir(), "nginx.pid")
	if running, known := hostRunning(); running || !known {
		t.Errorf("no pid file: hostRunning = %v, %v, want stopped and known", running, known)
	}
	_ = os.WriteFile(pidFile, []byte(fmt.Sprint(os.Getpid())), 0o644)
	if running, _ := hostRunning(); !running {
		t.Error("a live pid read as stopped")
	}
	_ = os.WriteFile(pidFile, []byte("999999"), 0o644)
	if running, _ := hostRunning(); running {
		t.Error("a dead pid read as running")
	}
}

// On the host, lerd-nginx is a launchd job running the host's nginx, and the
// container it replaces is removed: it holds the loopback port nginx listens on.
func TestRewriteNginxQuadlet_onHostWritesTheHostUnitAndDropsTheContainer(t *testing.T) {
	hostModeLegs(t)
	var wrote, removed string
	prevWrite, prevRemove := writeServiceUnitFn, removeContainerFn
	t.Cleanup(func() { writeServiceUnitFn, removeContainerFn = prevWrite, prevRemove })
	writeServiceUnitFn = func(name, content string) (bool, error) { wrote = name + "\n" + content; return true, nil }
	removeContainerFn = func(name string) { removed = name }

	changed, err := RewriteNginxQuadlet()
	if err != nil || !changed {
		t.Fatalf("RewriteNginxQuadlet = %v, %v; want changed", changed, err)
	}
	if !strings.HasPrefix(wrote, "lerd-nginx\n") || !strings.Contains(wrote, "ExecStart=/opt/homebrew/bin/nginx -c ") {
		t.Errorf("host unit not written as lerd-nginx:\n%s", wrote)
	}
	if removed != "lerd-nginx" {
		t.Errorf("container removed = %q, want lerd-nginx", removed)
	}
}

// A native install that predates host mode already has the upstreams file in
// its container form; switching has to rewrite it, not find it current.
func TestEnsureNativeUpstreams_rewritesAContainerFormFileOnHost(t *testing.T) {
	hostModeLegs(t)
	path := filepath.Join(config.NginxConfD(), nativeUpstreamsFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(renderNativeUpstreams()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureNativeUpstreamsFor([]byte("fastcgi_pass lerd_native_php84;")); err != nil {
		t.Fatal(err)
	}
	if got := readConf(t, path); strings.Contains(got, "host.containers.internal") {
		t.Errorf("upstreams left in container form on the host:\n%s", got)
	}
}
