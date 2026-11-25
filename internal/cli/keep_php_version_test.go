package cli

import (
	"errors"
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

// stubResumePHP fakes everything bringing a PHP version up touches and returns
// the versions asked for. imageExists answers for a build that fails.
func stubResumePHP(t *testing.T, buildErr error, imageExists bool) *[]string {
	t.Helper()
	prevReload, prevWrite, prevBuild, prevXdebug, prevStart, prevRestart, prevImage, prevEnvDir :=
		pauseNginxReload, writeFPMQuadlet, buildFPMImageTo, ensureXdebugIni, startUnitFn, restartUnitFn, fpmImageExistsFn, ensureEnvDirFn
	t.Cleanup(func() {
		pauseNginxReload, writeFPMQuadlet, buildFPMImageTo, ensureXdebugIni, startUnitFn, restartUnitFn, fpmImageExistsFn, ensureEnvDirFn =
			prevReload, prevWrite, prevBuild, prevXdebug, prevStart, prevRestart, prevImage, prevEnvDir
	})
	var ensured []string
	pauseNginxReload = func(string) {}
	writeFPMQuadlet = func(v string) (bool, error) { ensured = append(ensured, v); return false, nil }
	buildFPMImageTo = func(string, bool, io.Writer) (bool, error) { return false, buildErr }
	ensureXdebugIni = func(string) error { return nil }
	startUnitFn = func(string) error { return nil }
	restartUnitFn = func(string) error { return nil }
	ensureEnvDirFn = func() {}
	fpmImageExistsFn = func(string) bool { return imageExists }
	return &ensured
}

func pausedSite(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubInstalledPHP(t, "8.4", "8.5")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"php":"^8.2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Path: dir, Domains: []string{"shop.test"}, PHPVersion: "8.3", Paused: true}); err != nil {
		t.Fatal(err)
	}
}

// PHP 8.3 was removed while its only site was paused: resuming installs it
// again instead of moving the site to the newest version that fits.
func TestUnpauseSite_reinstallsARemovedPHP(t *testing.T) {
	pausedSite(t)
	ensured := stubResumePHP(t, nil, true)
	if err := UnpauseSite("shop"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*ensured, ",") != "8.3" {
		t.Errorf("ensured PHP %v, want only 8.3", *ensured)
	}
	if conf, _ := os.ReadFile(filepath.Join(config.NginxConfD(), "shop.test.conf")); !strings.Contains(string(conf), "php83") && !strings.Contains(string(conf), ":9483") {
		t.Errorf("vhost does not point at PHP 8.3:\n%s", conf)
	}
}

// With no image to serve from, a failed build leaves the site paused with its
// landing page rather than live and answering 502. An image that is only out of
// date can still serve, so that failure does not block the resume.
func TestUnpauseSite_failedBuild(t *testing.T) {
	for _, imageExists := range []bool{false, true} {
		pausedSite(t)
		stubResumePHP(t, errors.New("offline"), imageExists)
		err := UnpauseSite("shop")
		site, findErr := config.FindSite("shop")
		if findErr != nil {
			t.Fatal(findErr)
		}
		_, statErr := os.Stat(filepath.Join(config.NginxConfD(), "shop.test.conf"))
		if (err != nil) == imageExists || site.Paused == imageExists || (statErr == nil) != imageExists {
			t.Errorf("imageExists=%v: err=%v paused=%v vhost err=%v", imageExists, err, site.Paused, statErr)
		}
	}
}

// A worktree pinned to a version that cannot be brought up stops the resume
// before any route is restored, so neither it nor the site is left on a 502.
func TestUnpauseSite_worktreePHPUnavailable(t *testing.T) {
	pausedSite(t)
	stubResumePHP(t, nil, false)
	site, err := config.FindSite("shop")
	if err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	meta := filepath.Join(site.Path, ".git", "worktrees", "feat")
	if err := os.MkdirAll(meta, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(meta, "HEAD"):     "ref: refs/heads/feat\n",
		filepath.Join(meta, "gitdir"):   filepath.Join(wt, ".git") + "\n",
		filepath.Join(wt, ".git"):       "gitdir: " + meta + "\n",
		filepath.Join(wt, ".lerd.yaml"): "php_version: \"8.1\"\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	buildFPMImageTo = func(v string, _ bool, _ io.Writer) (bool, error) {
		if v == "8.1" {
			return false, errors.New("offline")
		}
		return false, nil
	}

	err = UnpauseSite("shop")
	if err == nil || !strings.Contains(err.Error(), "8.1") {
		t.Errorf("err = %v, want it to name PHP 8.1", err)
	}
	if site, _ := config.FindSite("shop"); site == nil || !site.Paused {
		t.Error("the site must stay paused")
	}
	if confs, _ := filepath.Glob(filepath.Join(config.NginxConfD(), "*shop.test.conf")); len(confs) != 0 {
		t.Errorf("routes were restored: %v", confs)
	}
}

// A pin changed while the site was paused is saved on resume, or commands would
// stay on the old version while the web runs the new one.
func TestUnpauseSite_savesAPinChangedWhilePaused(t *testing.T) {
	pausedSite(t)
	stubResumePHP(t, nil, true)
	site, err := config.FindSite("shop")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(site.Path, ".php-version"), []byte("8.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UnpauseSite("shop"); err != nil {
		t.Fatal(err)
	}
	if site, _ := config.FindSite("shop"); site.PHPVersion != "8.4" || site.Paused {
		t.Errorf("registry has PHP %s, paused=%v, want 8.4 resumed", site.PHPVersion, site.Paused)
	}
	if v, _ := phpDet.VersionForDir(site.Path); v != "8.4" {
		t.Errorf("commands run on PHP %s, want 8.4", v)
	}
}

// Resuming clamps to the framework's range, as linking does.
func TestUnpauseSite_keepsTheFrameworkRange(t *testing.T) {
	pausedSite(t)
	ensured := stubResumePHP(t, nil, true)
	site, err := config.FindSite("shop")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.StoreFrameworksDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(config.StoreFrameworksDir(), "acme@1.yaml"): "name: acme\nlabel: Acme\nversion: \"1\"\npublic_dir: public\nphp:\n  min: \"8.3\"\n  max: \"8.4\"\ndetect:\n  - composer: acme/framework\n",
		filepath.Join(site.Path, "composer.json"):                 `{"require":{"acme/framework":"^1.0"}}`,
		filepath.Join(site.Path, ".php-version"):                  "8.5\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	site.Framework = "acme"
	if err := config.AddSite(*site); err != nil {
		t.Fatal(err)
	}
	if err := UnpauseSite("shop"); err != nil {
		t.Fatal(err)
	}
	if site, _ := config.FindSite("shop"); site.PHPVersion != "8.3" || strings.Join(*ensured, ",") != "8.3" {
		t.Errorf("registry %s, ensured %v, want 8.3 kept", site.PHPVersion, *ensured)
	}
}

// A custom-FPM site's version comes from its Containerfile, never from detection.
func TestResumePHPVersion_customFPMIsNotRedetected(t *testing.T) {
	pausedSite(t)
	site, err := config.FindSite("shop")
	if err != nil {
		t.Fatal(err)
	}
	site.Runtime = "fpm-custom"
	if !site.IsCustomFPM() {
		t.Fatal("fixture is not a custom-FPM site")
	}
	if err := os.WriteFile(filepath.Join(site.Path, ".php-version"), []byte("8.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resumePHPVersion(site); got != "8.3" {
		t.Errorf("resumePHPVersion = %s, want the Containerfile's 8.3", got)
	}
}

// Linking sets a framework's range aside when the project's own composer.json
// rules all of it out, and resuming must agree or the app cannot boot.
func TestUnpauseSite_projectOverrulesTheFrameworkRange(t *testing.T) {
	pausedSite(t)
	ensured := stubResumePHP(t, nil, true)
	site, err := config.FindSite("shop")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.StoreFrameworksDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(config.StoreFrameworksDir(), "acme@1.yaml"): "name: acme\nlabel: Acme\nversion: \"1\"\npublic_dir: public\nphp:\n  min: \"8.1\"\n  max: \"8.2\"\ndetect:\n  - composer: acme/framework\n",
		filepath.Join(site.Path, "composer.json"):                 `{"require":{"php":">=8.3","acme/framework":"^1.0"}}`,
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	site.Framework = "acme"
	if err := config.AddSite(*site); err != nil {
		t.Fatal(err)
	}
	if err := UnpauseSite("shop"); err != nil {
		t.Fatal(err)
	}
	if site, _ := config.FindSite("shop"); site.PHPVersion != "8.3" || strings.Join(*ensured, ",") != "8.3" {
		t.Errorf("registry %s, ensured %v, want 8.3", site.PHPVersion, *ensured)
	}
}
