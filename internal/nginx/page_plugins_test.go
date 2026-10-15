package nginx

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/plugins"
)

// pageSite registers a site with the given page plugins switched on and
// installs a page-script plugin called hello.
func pageSite(t *testing.T, pagePlugins ...string) config.Site {
	t.Helper()
	if err := plugins.Scaffold("hello", filepath.Join(plugins.Dir(), "hello")); err != nil {
		t.Fatal(err)
	}
	site := config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: t.TempDir(), PagePlugins: pagePlugins}
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}
	return site
}

func TestGenerateVhost_PagePluginInjectsScriptAndProxy(t *testing.T) {
	confD := setupConfD(t)
	site := pageSite(t, "hello")
	if err := GenerateVhost(site, "8.3"); err != nil {
		t.Fatal(err)
	}
	content := readConf(t, filepath.Join(confD, "myapp.test.conf"))
	for _, want := range []string{
		`sub_filter '</head>' '<script src="/_lerd/plugins/hello/page.js" defer></script></head>';`,
		"location ^~ /_lerd/plugins/ {",
		"proxy_pass " + lerdUIUpstream() + ";",
		`proxy_set_header X-Lerd-Site "myapp";`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in:\n%s", want, content)
		}
	}
}

// nginx rewrites </head> once, so browser logs and plugin scripts must share
// the one substitution or whichever comes second never reaches the page.
func TestGenerateVhost_PagePluginSharesTheHeadWithBrowserLogs(t *testing.T) {
	confD := setupConfD(t)
	pageSite(t, "hello")
	site := captureSite(t, config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: t.TempDir(), PagePlugins: []string{"hello"}}, true, true)
	if err := GenerateVhost(site, "8.3"); err != nil {
		t.Fatal(err)
	}
	content := readConf(t, filepath.Join(confD, "myapp.test.conf"))
	if n := strings.Count(content, "sub_filter '</head>'"); n != 1 {
		t.Fatalf("want one </head> substitution, got %d:\n%s", n, content)
	}
	want := `sub_filter '</head>' '<script src="/_lerd/browser.js" data-rid="$upstream_http_x_lerd_rid"></script><script src="/_lerd/plugins/hello/page.js" defer></script></head>';`
	if !strings.Contains(content, want) {
		t.Errorf("missing %q in:\n%s", want, content)
	}
}

// Nothing is injected for a plugin the site did not switch on, or one that is
// no longer installed.
func TestGenerateVhost_PagePluginNeedsOptInAndInstall(t *testing.T) {
	for name, enabled := range map[string][]string{"not switched on": nil, "not installed": {"gone"}} {
		t.Run(name, func(t *testing.T) {
			confD := setupConfD(t)
			site := pageSite(t, enabled...)
			if err := GenerateVhost(site, "8.3"); err != nil {
				t.Fatal(err)
			}
			content := readConf(t, filepath.Join(confD, "myapp.test.conf"))
			if strings.Contains(content, "sub_filter") || strings.Contains(content, "/_lerd/plugins/") {
				t.Errorf("vhost must be untouched:\n%s", content)
			}
		})
	}
}
