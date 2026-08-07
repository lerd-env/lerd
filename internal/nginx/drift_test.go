package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func fpmSite() config.Site {
	return config.Site{
		Name:       "myapp",
		Domains:    []string{"myapp.test"},
		Path:       "/srv/myapp",
		PHPVersion: "8.4",
	}
}

// A vhost lerd wrote and nothing has changed since is not drifted.
func TestVhostDrift_freshlyGeneratedVhostIsCurrent(t *testing.T) {
	setupConfD(t)
	site := fpmSite()
	if err := GenerateVhost(site, "8.4"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}

	drift, err := VhostDrift(site)
	if err != nil {
		t.Fatalf("VhostDrift: %v", err)
	}
	if drift.Drifted {
		t.Errorf("a just-written vhost reads as drifted: %s", drift.Detail)
	}
}

// The whole point: a vhost written before something changed no longer says what
// lerd would write, and the difference has to be named rather than merely
// flagged.
func TestVhostDrift_staleVhostIsReportedWithWhatChanged(t *testing.T) {
	confD := setupConfD(t)
	site := fpmSite()
	if err := GenerateVhost(site, "8.3"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}

	site.PHPVersion = "8.4"
	drift, err := VhostDrift(site)
	if err != nil {
		t.Fatalf("VhostDrift: %v", err)
	}
	if !drift.Drifted {
		t.Fatal("a vhost still pointing at the old PHP container reads as current")
	}
	if !strings.Contains(drift.Detail, "php83") || !strings.Contains(drift.Detail, "php84") {
		t.Errorf("detail = %q, want both the old and the wanted fastcgi container", drift.Detail)
	}
	if drift.Path != filepath.Join(confD, "myapp.test.conf") {
		t.Errorf("path = %q, want the site's conf", drift.Path)
	}
}

// A site with no vhost at all is drifted too: the file that should exist does
// not, which is the same finding and the same fix.
func TestVhostDrift_missingVhostIsDrift(t *testing.T) {
	setupConfD(t)

	drift, err := VhostDrift(fpmSite())
	if err != nil {
		t.Fatalf("VhostDrift: %v", err)
	}
	if !drift.Drifted || !strings.Contains(drift.Detail, "no vhost") {
		t.Errorf("drift = %+v, want a missing-vhost finding", drift)
	}
}

// A paused site serves its landing page and an idle-suspended host proxy serves
// the waking page. Both are files lerd put there on purpose, so neither is
// drift and neither may be regenerated out from under the state it belongs to.
func TestVhostDrift_deliberateVhostsAreNotDrift(t *testing.T) {
	setupConfD(t)

	paused := fpmSite()
	paused.Paused = true
	if drift, err := VhostDrift(paused); err != nil || drift.Checked {
		t.Errorf("paused site: drift = %+v, err = %v, want unchecked", drift, err)
	}

	waking := config.Site{
		Name:                 "proxyapp",
		Domains:              []string{"proxyapp.test"},
		Path:                 "/srv/proxyapp",
		HostPort:             3000,
		HostCommand:          "npm run dev",
		IdleSuspendedWorkers: []string{"app"},
	}
	if drift, err := VhostDrift(waking); err != nil || drift.Checked {
		t.Errorf("waking host proxy: drift = %+v, err = %v, want unchecked", drift, err)
	}
}

// Regenerating writes the current vhost over the drifted one and leaves nothing
// to report.
func TestRegenerateVhost_repairsTheDrift(t *testing.T) {
	confD := setupConfD(t)
	site := fpmSite()
	if err := GenerateVhost(site, "8.3"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	site.PHPVersion = "8.4"

	if err := RegenerateVhost(site); err != nil {
		t.Fatalf("RegenerateVhost: %v", err)
	}

	content := readConf(t, filepath.Join(confD, "myapp.test.conf"))
	if !strings.Contains(content, "lerd-php84-fpm") {
		t.Errorf("regenerated vhost still not on php84:\n%s", content)
	}
	drift, err := VhostDrift(site)
	if err != nil {
		t.Fatalf("VhostDrift: %v", err)
	}
	if drift.Drifted {
		t.Errorf("still drifted after regenerating: %s", drift.Detail)
	}
}

// A secured site's vhost is the SSL render, since that is what the secure flow
// leaves at <domain>.conf, so it must not read as permanently drifted.
func TestVhostDrift_securedSiteComparesTheSSLRender(t *testing.T) {
	confD := setupConfD(t)
	certs := filepath.Join(t.TempDir(), "certs")
	if err := os.MkdirAll(certs, 0o755); err != nil {
		t.Fatal(err)
	}

	site := fpmSite()
	site.Secured = true
	if err := RegenerateVhost(site); err != nil {
		t.Fatalf("RegenerateVhost: %v", err)
	}
	content := readConf(t, filepath.Join(confD, "myapp.test.conf"))
	if !strings.Contains(content, "listen 443") {
		t.Fatalf("secured site's conf is not the SSL render:\n%s", content)
	}

	drift, err := VhostDrift(site)
	if err != nil {
		t.Fatalf("VhostDrift: %v", err)
	}
	if drift.Drifted {
		t.Errorf("secured site reads as drifted right after regenerating: %s", drift.Detail)
	}
}
