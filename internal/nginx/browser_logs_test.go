package nginx

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestGenerateVhost_BrowserLogsOnInjectsScriptAndEndpoint(t *testing.T) {
	confD := setupConfD(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	site := captureSite(t, config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: t.TempDir(), Secured: true}, true, true)
	for _, gen := range []func(config.Site, string) error{GenerateVhost, GenerateSSLVhost} {
		if err := gen(site, "8.3"); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"myapp.test.conf", "myapp.test-ssl.conf"} {
		content := readConf(t, filepath.Join(confD, name))
		for _, want := range []string{
			`sub_filter '</head>' '<script src="/_lerd/browser.js" data-rid="$upstream_http_x_lerd_rid"></script></head>';`,
			"add_header Access-Control-Expose-Headers X-Lerd-Rid always;",
			"location = /_lerd/browser {",
			"location = /_lerd/browser.js {",
			"proxy_pass " + lerdUIUpstream() + "/_lerd/browser;",
			"proxy_pass " + lerdUIUpstream() + "/_lerd/browser.js;",
			`proxy_set_header X-Lerd-Site "myapp";`,
		} {
			if !strings.Contains(content, want) {
				t.Errorf("%s: missing %q in:\n%s", name, want, content)
			}
		}
	}
}

// The script goes in only while debug capture is on and the site opted in.
func TestGenerateVhost_BrowserLogsNeedsDebugAndTheSite(t *testing.T) {
	for _, tc := range []struct {
		name          string
		debug, siteOn bool
	}{
		{"debug off", false, true},
		{"site not opted in", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			confD := setupConfD(t)
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			site := captureSite(t, config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: t.TempDir()}, tc.debug, tc.siteOn)
			if err := GenerateVhost(site, "8.3"); err != nil {
				t.Fatal(err)
			}
			content := readConf(t, filepath.Join(confD, "myapp.test.conf"))
			if strings.Contains(content, "sub_filter") || strings.Contains(content, "/_lerd/browser") {
				t.Errorf("capture off must not touch the vhost:\n%s", content)
			}
		})
	}
}

// captureSite registers site with the debug switch and its own capture opt-in
// set as given; siteOn is ignored when the site already carries settings.
func captureSite(t *testing.T, site config.Site, debug, siteOn bool) config.Site {
	t.Helper()
	cfg, _ := config.LoadGlobal()
	cfg.SetDumpsEnabled(debug)
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	if site.BrowserLogs == nil {
		site.BrowserLogs = &config.BrowserLogs{Enabled: &siteOn}
	}
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}
	return site
}

// A host-proxy site gets the same block, and asks its dev server for an
// uncompressed body, since sub_filter cannot rewrite a compressed one.
func TestGenerateHostProxyVhost_BrowserLogsAsksForPlainBody(t *testing.T) {
	confD := setupConfD(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	site := captureSite(t, config.Site{Name: "spa", Domains: []string{"spa.test"}, Path: t.TempDir(), HostPort: 5173}, true, true)
	if err := GenerateHostProxyVhost(site); err != nil {
		t.Fatal(err)
	}
	content := readConf(t, filepath.Join(confD, "spa.test.conf"))
	for _, want := range []string{
		`<script src="/_lerd/browser.js" data-rid="$upstream_http_x_lerd_rid"></script>`,
		"location = /_lerd/browser {",
		`proxy_set_header X-Lerd-Site "spa";`,
		`proxy_set_header Accept-Encoding "";`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in:\n%s", want, content)
		}
	}
}
