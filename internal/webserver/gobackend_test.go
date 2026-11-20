package webserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

func useGo(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	cfg, _ := config.LoadGlobal()
	cfg.WebServer = config.WebServerGo
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
}

// With the Go server chosen, its unit replaces nginx's under the same name, so
// the LAN bind policy, logs and health checks follow it, and while LAN is off
// it publishes on loopback only.
func TestGoBackend_unitRunsLerdWebServeOnLoopback(t *testing.T) {
	useGo(t)
	if _, err := RewriteUnit(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(config.QuadletDir(), podman.WebServerUnit+".container"))
	if err != nil {
		t.Fatal(err)
	}
	unit := string(data)
	exe, _ := os.Executable()
	for _, want := range []string{
		"ContainerName=" + podman.WebServerUnit,
		"Exec=/usr/local/bin/lerd web-serve",
		"Volume=" + exe + ":/usr/local/bin/lerd:ro",
		"Environment=XDG_DATA_HOME=" + filepath.Dir(config.DataDir()),
		"Environment=XDG_CONFIG_HOME=" + filepath.Dir(config.ConfigDir()),
		"Volume=" + config.RunDir() + ":" + config.RunDir() + ":rw",
		"PublishPort=127.0.0.1:80:80",
		"PublishPort=127.0.0.1:443:443",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit lacks %q:\n%s", want, unit)
		}
	}
	if strings.Contains(unit, "nginx:alpine") {
		t.Error("the Go unit still runs the nginx image")
	}
}

// The Go server reads the registry itself, so writing a site's config is not
// its job, and asking whether it serves what was written is always yes.
func TestGoBackend_hasNoFilesToWrite(t *testing.T) {
	useGo(t)
	site := config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: "/srv/myapp", PHPVersion: "8.4"}
	if err := ApplySite(site); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(config.NginxConfD(), "myapp.test.conf")); !os.IsNotExist(err) {
		t.Error("the Go backend wrote an nginx vhost")
	}
	if d, _ := Drift(site); d.Checked {
		t.Error("drift checked against vhost files the Go server never reads")
	}
}

// Linking a site rewrites the FPM units and, beside them, the web server's
// unit with any new mount. That rewrite must keep the chosen server: it once
// put nginx back under a running Go server on the first link.
func TestGoBackend_podmanRewritesKeepTheGoUnit(t *testing.T) {
	useGo(t)
	content, err := podman.WebServerUnitContent([]string{"/srv/extra"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "lerd web-serve") || strings.Contains(content, "nginx:alpine") {
		t.Fatalf("podman would write:\n%s", content)
	}
	if !strings.Contains(content, "/srv/extra") {
		t.Errorf("the extra mount is lost:\n%s", content)
	}
}
