package nginx

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestGenerateVhost_BrowserCaptureOnInjectsScriptAndEndpoint(t *testing.T) {
	confD := setupConfD(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, _ := config.LoadGlobal()
	cfg.BrowserCapture.Enabled = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	site := config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: "/srv/myapp", Secured: true}
	for _, gen := range []func(config.Site, string) error{GenerateVhost, GenerateSSLVhost} {
		if err := gen(site, "8.3"); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"myapp.test.conf", "myapp.test-ssl.conf"} {
		content := readConf(t, filepath.Join(confD, name))
		for _, want := range []string{
			`sub_filter '</head>' '<script src="/_lerd/browser.js"></script></head>';`,
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

// A site that uses /_lerd itself moves capture to its own route; lerd-ui
// still gets the fixed path.
func TestGenerateVhost_BrowserCaptureUsesTheSiteRoute(t *testing.T) {
	confD := setupConfD(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, _ := config.LoadGlobal()
	cfg.BrowserCapture.Enabled = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	site := config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: t.TempDir(), BrowserCapture: &config.BrowserCapture{Route: "/__dev/capture"}}
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}
	if err := GenerateVhost(site, "8.3"); err != nil {
		t.Fatal(err)
	}
	content := readConf(t, filepath.Join(confD, "myapp.test.conf"))
	for _, want := range []string{
		`<script src="/__dev/capture.js"></script>`,
		"location = /__dev/capture {",
		"location = /__dev/capture.js {",
		"proxy_pass " + lerdUIUpstream() + "/_lerd/browser.js;",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in:\n%s", want, content)
		}
	}
	if strings.Contains(content, "location = /_lerd/browser") {
		t.Errorf("default route still claimed:\n%s", content)
	}
}

func TestGenerateVhost_BrowserCaptureOffLeavesPageAlone(t *testing.T) {
	confD := setupConfD(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	site := config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: "/srv/myapp"}
	if err := GenerateVhost(site, "8.3"); err != nil {
		t.Fatal(err)
	}
	content := readConf(t, filepath.Join(confD, "myapp.test.conf"))
	if strings.Contains(content, "sub_filter") || strings.Contains(content, "/_lerd/browser") {
		t.Errorf("capture off must not touch the vhost:\n%s", content)
	}
}

// A host-proxy site gets the same block, and asks its dev server for an
// uncompressed body, since sub_filter cannot rewrite a compressed one.
func TestGenerateHostProxyVhost_BrowserCaptureAsksForPlainBody(t *testing.T) {
	confD := setupConfD(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, _ := config.LoadGlobal()
	cfg.BrowserCapture.Enabled = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	site := config.Site{Name: "spa", Domains: []string{"spa.test"}, Path: t.TempDir(), HostPort: 5173}
	if err := GenerateHostProxyVhost(site); err != nil {
		t.Fatal(err)
	}
	content := readConf(t, filepath.Join(confD, "spa.test.conf"))
	for _, want := range []string{
		`<script src="/_lerd/browser.js"></script>`,
		"location = /_lerd/browser {",
		`proxy_set_header X-Lerd-Site "spa";`,
		`proxy_set_header Accept-Encoding "";`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in:\n%s", want, content)
		}
	}
}
