package nginx

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A containerised site keeps talking to the shared FPM container on 9000; a
// native site talks to the host listener for its PHP version instead.
func nativeMode(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.PHP.Runtime = config.PHPRuntimeNative
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
}

func TestFPMUpstream(t *testing.T) {
	nativeMode(t)
	cases := []struct {
		name     string
		site     *config.Site
		version  string
		wantHost string
		wantPort int
	}{
		{"frankenphp is never moved to the host", &config.Site{Name: "shop", Runtime: "frankenphp"}, "8.4", "lerd-php84-fpm", 9000},
		{"native", &config.Site{Name: "shop"}, "8.4", "host.containers.internal", 9484},
		{"native other version", &config.Site{Name: "shop"}, "8.1", "host.containers.internal", 9481},
	}
	for _, c := range cases {
		host, port := fpmUpstream(c.site, c.version)
		if host != c.wantHost || port != c.wantPort {
			t.Errorf("%s: fpmUpstream = (%q,%d), want (%q,%d)", c.name, host, port, c.wantHost, c.wantPort)
		}
	}
}

// An unparseable version must not silently produce port 0 and a dead vhost.
func TestFPMUpstreamFallsBackOnBadVersion(t *testing.T) {
	nativeMode(t)
	_, port := fpmUpstream(&config.Site{Name: "shop"}, "nonsense")
	if port != 9000 {
		t.Errorf("bad version should fall back to the container port, got %d", port)
	}
}

func TestGenerateVhost_nativeSiteFastcgisToLoopback(t *testing.T) {
	confD := setupConfD(t)
	nativeMode(t)
	site := config.Site{
		Name:    "nativeapp",
		Domains: []string{"nativeapp.test"},
		Path:    "/srv/nativeapp",
	}
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	content := readConf(t, filepath.Join(confD, "nativeapp.test.conf"))
	for _, want := range []string{"fastcgi_pass lerd_native_php84;", "fastcgi_keep_conn on;"} {
		if !strings.Contains(content, want) {
			t.Errorf("native vhost should reuse its PHP connections (%q), got:\n%s", want, content)
		}
	}
	upstreams := readConf(t, filepath.Join(confD, nativeUpstreamsFile))
	for _, want := range []string{"upstream lerd_native_php84 {", "server host.containers.internal:9484;", "keepalive 8;", "keepalive_timeout 10s;"} {
		if !strings.Contains(upstreams, want) {
			t.Errorf("native upstreams missing %q:\n%s", want, upstreams)
		}
	}
	if strings.Contains(content, "lerd-php84-fpm") {
		t.Errorf("native vhost must not reference the FPM container:\n%s", content)
	}
}

// The container path must keep emitting exactly what it always did.
func TestGenerateVhost_containerSiteKeepsPort9000(t *testing.T) {
	confD := setupConfD(t)
	site := config.Site{Name: "plainapp", Domains: []string{"plainapp.test"}, Path: "/srv/plainapp"}
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	content := readConf(t, filepath.Join(confD, "plainapp.test.conf"))
	if !strings.Contains(content, "fastcgi_pass $fpm:9000;") {
		t.Errorf("container vhost must still use 9000, got:\n%s", content)
	}
}

// nginx refuses the whole config over one vhost naming an upstream nobody
// defined, so a version the upstreams file does not list keeps the old form.
func TestGenerateVhost_nativeSiteOnAnUnlistedVersionKeepsAPlainPass(t *testing.T) {
	confD := setupConfD(t)
	nativeMode(t)
	site := config.Site{Name: "oddapp", Domains: []string{"oddapp.test"}, Path: "/srv/oddapp"}
	if err := GenerateVhost(site, "9.9"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	content := readConf(t, filepath.Join(confD, "oddapp.test.conf"))
	if strings.Contains(content, "lerd_native_php") || !strings.Contains(content, "fastcgi_pass $fpm:9499;") {
		t.Errorf("an unlisted version should pass to its port directly, got:\n%s", content)
	}
}

// Every version a site can run has its upstream, so the file never has to be
// rewritten when a site switches PHP version.
func TestNativeUpstreamsCoverEverySupportedVersion(t *testing.T) {
	got := renderNativeUpstreams()
	for _, v := range config.SupportedPHPVersions {
		if !strings.Contains(got, "upstream "+nativeUpstreamName(v)+" {") {
			t.Errorf("no upstream for PHP %s:\n%s", v, got)
		}
	}
}

// A worktree's vhost can be the first native one written, after an upgrade or
// when only its dev server starts, so it writes the upstreams it names too.
func TestGenerateWorktreeVhost_nativeWritesTheUpstreamsItNames(t *testing.T) {
	confD := setupConfD(t)
	nativeMode(t)
	if err := GenerateWorktreeVhost("feat.nativeapp.test", "/srv/nativeapp-feat", "8.4", "nativeapp", "feat"); err != nil {
		t.Fatalf("GenerateWorktreeVhost: %v", err)
	}
	content := readConf(t, filepath.Join(confD, "feat.nativeapp.test.conf"))
	if !strings.Contains(content, "fastcgi_pass lerd_native_php84;") {
		t.Fatalf("native worktree vhost should pass to the 8.4 upstream, got:\n%s", content)
	}
	if !strings.Contains(readConf(t, filepath.Join(confD, nativeUpstreamsFile)), "upstream lerd_native_php84 {") {
		t.Error("the upstream the worktree vhost names was not written")
	}
}
