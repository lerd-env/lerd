package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	phpDet "github.com/geodro/lerd/internal/php"
)

// stubInstalledPHP makes version detection see exactly these PHP versions.
func stubInstalledPHP(t *testing.T, versions ...string) {
	t.Helper()
	prev := phpDet.InstalledForRuntimeFn
	phpDet.InstalledForRuntimeFn = func() ([]string, error) { return versions, nil }
	t.Cleanup(func() { phpDet.InstalledForRuntimeFn = prev })
}

// Resuming a site rewrote its vhost for the newest PHP composer.json allowed,
// while the registry still said the version it had been served on (#2667).
func TestUnpauseSite_keepsThePHPVersionTheSiteIsServedOn(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubInstalledPHP(t, "8.4", "8.5")

	prevReload, prevWrite, prevBuild, prevXdebug, prevStart, prevRestart :=
		pauseNginxReload, writeFPMQuadlet, buildFPMImageTo, ensureXdebugIni, startUnitFn, restartUnitFn
	t.Cleanup(func() {
		pauseNginxReload, writeFPMQuadlet, buildFPMImageTo, ensureXdebugIni, startUnitFn, restartUnitFn =
			prevReload, prevWrite, prevBuild, prevXdebug, prevStart, prevRestart
	})
	pauseNginxReload = func(string) {}
	var ensured []string
	writeFPMQuadlet = func(v string) (bool, error) { ensured = append(ensured, v); return false, nil }
	buildFPMImageTo = func(string, bool, io.Writer) (bool, error) { return false, nil }
	ensureXdebugIni = func(string) error { return nil }
	startUnitFn = func(string) error { return nil }
	restartUnitFn = func(string) error { return nil }

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"php":"^8.4"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Path: dir, Domains: []string{"shop.test"}, PHPVersion: "8.4", Paused: true}); err != nil {
		t.Fatal(err)
	}

	if err := UnpauseSite("shop"); err != nil {
		t.Fatal(err)
	}
	vhost, err := os.ReadFile(filepath.Join(config.NginxConfD(), "shop.test.conf"))
	if err != nil {
		t.Fatal(err)
	}
	conf := string(vhost)
	if strings.Contains(conf, "php85") || strings.Contains(conf, ":9485") {
		t.Errorf("vhost was moved to PHP 8.5:\n%s", conf)
	}
	if !strings.Contains(conf, "php84") && !strings.Contains(conf, ":9484") {
		t.Errorf("vhost does not point at PHP 8.4:\n%s", conf)
	}
	for _, v := range ensured {
		if v != "8.4" {
			t.Errorf("unpause ensured an FPM for PHP %s, want only 8.4", v)
		}
	}
}
