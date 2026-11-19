package webserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Until a second backend exists, the current one is nginx, so a site applied
// through the seam lands in nginx's conf.d and removing it takes it back out.
func TestApplyAndRemoveSiteGoThroughNginx(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	site := config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: "/srv/myapp", PHPVersion: "8.4"}
	conf := filepath.Join(config.NginxConfD(), "myapp.test.conf")

	if err := ApplySite(site); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(conf)
	if err != nil || !strings.Contains(string(data), "server_name myapp.test") {
		t.Fatalf("ApplySite did not write the nginx vhost: %v\n%s", err, data)
	}
	if err := RemoveSite(site.PrimaryDomain()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(conf); !os.IsNotExist(err) {
		t.Fatalf("RemoveSite left %s behind", conf)
	}
}

// Worktree, paused and waking vhosts all go through the seam to nginx's
// conf.d, under the domain they serve.
func TestWorktreePausedAndWakingGoThroughNginx(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	site := config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: "/srv/myapp", PHPVersion: "8.4", HostPort: 5173}
	confD := config.NginxConfD()
	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(confD, name))
		if err != nil {
			t.Fatalf("%s not written: %v", name, err)
		}
		return string(data)
	}

	if err := ApplyWorktree("feat.myapp.test", "/srv/myapp-feat", "8.4", "myapp.test", "myapp", "feat", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read("feat.myapp.test.conf"), "listen 443") {
		t.Error("secured worktree vhost has no TLS server")
	}
	if err := ApplyHostProxyWorktree("dev.myapp.test", "/srv/myapp-dev", "myapp.test", 5174, false, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read("dev.myapp.test.conf"), ":5174") {
		t.Error("host-proxy worktree vhost does not proxy to its port")
	}
	if err := ApplyPausedWorktree("old.myapp.test", "myapp.test", config.PausedDir(), false); err != nil {
		t.Fatal(err)
	}
	read("old.myapp.test.conf")
	if err := ApplyPausedSite(site); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read("myapp.test.conf"), "paused.html") {
		t.Error("paused site is not served its landing page")
	}
	if err := ApplyWakingSite(site); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read("myapp.test.conf"), WakeHoldPath) {
		t.Error("waking site does not hold requests at the wake path")
	}
}

// A site the seam just served reads as current; once its vhost is gone, the
// drift check reports it and regenerating puts it back.
func TestDriftAndRegenerateGoThroughNginx(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	site := config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: "/srv/myapp", PHPVersion: "8.4"}
	if err := ApplySite(site); err != nil {
		t.Fatal(err)
	}
	if d, err := Drift(site); err != nil || !d.Checked || d.Drifted {
		t.Fatalf("freshly served site reads as drifted: %+v %v", d, err)
	}
	if err := os.Remove(filepath.Join(config.NginxConfD(), "myapp.test.conf")); err != nil {
		t.Fatal(err)
	}
	if d, _ := Drift(site); !d.Drifted {
		t.Fatal("missing vhost not reported as drift")
	}
	if err := RegenerateSite(site); err != nil {
		t.Fatal(err)
	}
	if d, _ := Drift(site); d.Drifted {
		t.Fatalf("regenerate left the site drifted: %s", d.Detail)
	}
}
