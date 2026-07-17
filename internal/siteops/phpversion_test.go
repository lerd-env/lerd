package siteops

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// phpVersionTestSite registers an unsecured site in a temp registry and returns
// a copy callers can hand to SetSitePHPVersion. tweak adjusts the runtime shape
// before registration.
func phpVersionTestSite(t *testing.T, tweak func(*config.Site)) *config.Site {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	fakePodmanOnPath(t)

	projectDir := filepath.Join(tmp, "app")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	site := config.Site{
		Name:       "app",
		Domains:    []string{"app.test"},
		Path:       projectDir,
		PHPVersion: "8.4",
	}
	if tweak != nil {
		tweak(&site)
	}
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}
	s := site
	return &s
}

// asFPM, asFrankenPHP and friends name the runtime shapes the funnel branches on.
func asFPM(*config.Site)               {}
func asFrankenPHP(s *config.Site)      { s.Runtime = "frankenphp" }
func asCustomContainer(s *config.Site) { s.ContainerPort = 8080 }
func asHostProxy(s *config.Site)       { s.HostPort = 3000 }

// stubPHPVersionDeps neutralises the nginx reload and pins the framework range,
// so a test drives the funnel's decisions rather than the store or a container.
func stubPHPVersionDeps(t *testing.T, min, max string) *int {
	t.Helper()
	reloads := 0
	origReload := nginxReloadFn
	origRange := frameworkPHPRange
	origGap := imageGapFn
	t.Cleanup(func() {
		nginxReloadFn = origReload
		frameworkPHPRange = origRange
		imageGapFn = origGap
	})
	nginxReloadFn = func() error { reloads++; return nil }
	frameworkPHPRange = func(*config.Site) (string, string) { return min, max }
	imageGapFn = func(string) imageGapResult { return imageGapResult{} }
	return &reloads
}

func readPHPVersionFile(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".php-version"))
	if err != nil {
		t.Fatalf("reading .php-version: %v", err)
	}
	return strings.TrimSpace(string(b))
}

// The parent-site path must land the version in all three places that can
// disagree: the .php-version pin, the site registry, and nginx.
func TestSetSitePHPVersion_appliesToParentSite(t *testing.T) {
	site := phpVersionTestSite(t, asFPM)
	reloads := stubPHPVersionDeps(t, "", "")

	res, err := SetSitePHPVersion(site, "8.2", PHPVersionOpts{})
	if err != nil {
		t.Fatalf("SetSitePHPVersion: %v", err)
	}

	if res.Version != "8.2" || res.Clamped {
		t.Errorf("result = %+v, want version 8.2 unclamped", res)
	}
	if got := readPHPVersionFile(t, site.Path); got != "8.2" {
		t.Errorf(".php-version = %q, want 8.2", got)
	}
	if site.PHPVersion != "8.2" {
		t.Errorf("site.PHPVersion = %q, want 8.2", site.PHPVersion)
	}
	stored, err := config.FindSite("app")
	if err != nil {
		t.Fatal(err)
	}
	if stored.PHPVersion != "8.2" {
		t.Errorf("registry PHPVersion = %q, want 8.2", stored.PHPVersion)
	}
	if *reloads != 1 {
		t.Errorf("nginx reloads = %d, want 1", *reloads)
	}
}

// The framework's supported range wins over the request. MCP skipped this
// check entirely before the funnel, so a site could pin a version the watcher
// clamped straight back on its next pass.
func TestSetSitePHPVersion_clampsToFrameworkRange(t *testing.T) {
	site := phpVersionTestSite(t, asFPM)
	stubPHPVersionDeps(t, "8.3", "8.5")

	res, err := SetSitePHPVersion(site, "8.1", PHPVersionOpts{})
	if err != nil {
		t.Fatalf("SetSitePHPVersion: %v", err)
	}

	if res.Version != "8.3" || res.Requested != "8.1" || !res.Clamped {
		t.Errorf("result = %+v, want 8.1 clamped to 8.3", res)
	}
	// The pin file must carry the clamped version, not the request: a
	// .php-version lerd overrides on every pass is a lie other tools trust.
	if got := readPHPVersionFile(t, site.Path); got != "8.3" {
		t.Errorf(".php-version = %q, want the clamped 8.3", got)
	}
	if site.PHPVersion != "8.3" {
		t.Errorf("site.PHPVersion = %q, want 8.3", site.PHPVersion)
	}
}

func TestSetSitePHPVersion_rejectsUnsupportedVersion(t *testing.T) {
	site := phpVersionTestSite(t, asFPM)
	stubPHPVersionDeps(t, "", "")

	if _, err := SetSitePHPVersion(site, "9.9", PHPVersionOpts{}); err == nil {
		t.Fatal("SetSitePHPVersion accepted an unsupported version")
	}
	if _, err := os.Stat(filepath.Join(site.Path, ".php-version")); !os.IsNotExist(err) {
		t.Error("a rejected version still wrote .php-version")
	}
}

// Runtimes that have no PHP version of their own must be refused before
// anything is written, not silently pinned.
func TestSetSitePHPVersion_rejectsRuntimesWithoutPHPVersion(t *testing.T) {
	for _, tc := range []struct {
		name  string
		shape func(*config.Site)
	}{
		{"custom container", asCustomContainer},
		{"host proxy", asHostProxy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			site := phpVersionTestSite(t, tc.shape)
			stubPHPVersionDeps(t, "", "")

			if _, err := SetSitePHPVersion(site, "8.2", PHPVersionOpts{}); err == nil {
				t.Fatalf("%s site accepted a PHP version", tc.name)
			}
			if _, err := os.Stat(filepath.Join(site.Path, ".php-version")); !os.IsNotExist(err) {
				t.Errorf("%s site still wrote .php-version", tc.name)
			}
		})
	}
}

// FrankenPHP publishes no image below 8.2. Building one normalizes the version
// up and runs a different PHP than the site reports, so the site falls back to
// FPM instead.
func TestSetSitePHPVersion_demotesFrankenPHPBelowMinimum(t *testing.T) {
	site := phpVersionTestSite(t, asFrankenPHP)
	stubPHPVersionDeps(t, "", "")

	origLC := podman.UnitLifecycle
	origDR := podman.DaemonReloadFn
	origStop := StopRuntimeWorkers
	origRecreate := RecreateFPMWorkers
	t.Cleanup(func() {
		podman.UnitLifecycle = origLC
		podman.DaemonReloadFn = origDR
		StopRuntimeWorkers = origStop
		RecreateFPMWorkers = origRecreate
	})
	podman.UnitLifecycle = &recordingLifecycle{}
	podman.DaemonReloadFn = func() error { return nil }
	StopRuntimeWorkers = func(*config.Site) []string { return nil }
	RecreateFPMWorkers = func(*config.Site, []string) {}

	res, err := SetSitePHPVersion(site, "8.1", PHPVersionOpts{})
	if err != nil {
		t.Fatalf("SetSitePHPVersion: %v", err)
	}

	if !res.Demoted {
		t.Error("result did not report the FrankenPHP demotion")
	}
	stored, err := config.FindSite("app")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Runtime != "" {
		t.Errorf("registry runtime = %q, want FPM", stored.Runtime)
	}
	if stored.PHPVersion != "8.1" {
		t.Errorf("registry PHPVersion = %q, want 8.1", stored.PHPVersion)
	}
}

// This is the whole point of the issue: moving a site to a version whose image
// never built part of the declared set must say so, at the moment of the move,
// instead of letting the site quietly lose chromium.
func TestSetSitePHPVersion_reportsWhatTheTargetImageLacks(t *testing.T) {
	site := phpVersionTestSite(t, asFPM)
	stubPHPVersionDeps(t, "", "")
	imageGapFn = func(v string) imageGapResult {
		if v == "8.3" {
			return imageGapResult{missing: []string{"chromium"}}
		}
		return imageGapResult{}
	}

	res, err := SetSitePHPVersion(site, "8.3", PHPVersionOpts{})
	if err != nil {
		t.Fatalf("SetSitePHPVersion: %v", err)
	}
	if !reflect.DeepEqual(res.Missing, []string{"chromium"}) {
		t.Errorf("Missing = %v, want [chromium]", res.Missing)
	}
	// The gap is a warning, not a refusal: a framework upgrade that forces a
	// version must not fail because of an unrelated package.
	if site.PHPVersion != "8.3" {
		t.Errorf("the switch was blocked by a reported gap: PHPVersion = %q", site.PHPVersion)
	}
}

// The common case in the field: the target's image was built before the user
// declared chromium, so it does not have it and a rebuild would. This is
// distinct from an image that tried and failed, because the fix differs.
func TestSetSitePHPVersion_reportsAnImageThatPredatesTheDeclaredSet(t *testing.T) {
	site := phpVersionTestSite(t, asFPM)
	stubPHPVersionDeps(t, "", "")
	imageGapFn = func(string) imageGapResult { return imageGapResult{stale: true} }

	res, err := SetSitePHPVersion(site, "8.3", PHPVersionOpts{})
	if err != nil {
		t.Fatalf("SetSitePHPVersion: %v", err)
	}
	if !res.Stale {
		t.Error("result did not report that the image predates the declared set")
	}
	if res.NotInstalled || len(res.Missing) != 0 {
		t.Errorf("a stale image must not read as uninstalled or unbuildable: %+v", res)
	}
}

// An uninstalled version is a different problem, and reporting the whole
// declared set as "missing" from an image that does not exist is its own lie.
func TestSetSitePHPVersion_reportsAnUninstalledVersionSeparately(t *testing.T) {
	site := phpVersionTestSite(t, asFPM)
	stubPHPVersionDeps(t, "", "")
	imageGapFn = func(string) imageGapResult { return imageGapResult{notInstalled: true} }

	res, err := SetSitePHPVersion(site, "8.1", PHPVersionOpts{})
	if err != nil {
		t.Fatalf("SetSitePHPVersion: %v", err)
	}
	if !res.NotInstalled {
		t.Error("result did not report that the version has no image yet")
	}
	if len(res.Missing) != 0 {
		t.Errorf("Missing = %v, want empty for a version with no image", res.Missing)
	}
}

// imageGap holds the three-way decision, so it is worth driving directly:
// nothing declared means nothing to warn about, and a stale image must win over
// a realised record left behind by an older build.
func TestImageGap(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	origStale, origExists := imageStaleFn, imageExistsFn
	t.Cleanup(func() { imageStaleFn = origStale; imageExistsFn = origExists })
	imageExistsFn = func(v string) bool { return v == "8.4" }

	// A user who declares nothing has no set to fall short of, so an installed
	// version reports no gap however its images are labelled.
	imageStaleFn = func(string) bool { return true }
	if gap := imageGap("8.4"); gap.stale || gap.notInstalled || len(gap.missing) > 0 {
		t.Errorf("nothing declared should mean no gap on an installed version, got %+v", gap)
	}

	// But an unbuilt version must be reported whatever the declared set is:
	// most users declare nothing, and a site moved onto a version with no image
	// 502s on every request until someone builds it.
	if gap := imageGap("8.0"); !gap.notInstalled {
		t.Errorf("an unbuilt version was not reported for a user who declares nothing, got %+v", gap)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AddPackage("chromium")
	cfg.SetRealised("8.4", config.RealisedPHPSet{Packages: []string{"chromium"}})
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}

	// A stale image is reported as stale even though a realised record exists:
	// the record describes an older build and must not mask the drift.
	if gap := imageGap("8.4"); !gap.stale {
		t.Errorf("a stale image was not reported, got %+v", gap)
	}

	imageStaleFn = func(string) bool { return false }
	if gap := imageGap("8.4"); gap.stale || len(gap.missing) > 0 {
		t.Errorf("a current image carrying the set should have no gap, got %+v", gap)
	}
}

// A FrankenPHP site staying on a supported version keeps its runtime.
func TestSetSitePHPVersion_keepsFrankenPHPOnSupportedVersion(t *testing.T) {
	site := phpVersionTestSite(t, asFrankenPHP)
	stubPHPVersionDeps(t, "", "")

	origFinish := finishFrankenPHPFn
	t.Cleanup(func() { finishFrankenPHPFn = origFinish })
	finished := 0
	finishFrankenPHPFn = func(config.Site) error { finished++; return nil }

	res, err := SetSitePHPVersion(site, "8.3", PHPVersionOpts{})
	if err != nil {
		t.Fatalf("SetSitePHPVersion: %v", err)
	}

	if res.Demoted {
		t.Error("8.3 is a supported FrankenPHP version but the site was demoted")
	}
	if finished != 1 {
		t.Errorf("FrankenPHP re-link calls = %d, want 1", finished)
	}
	if site.Runtime != "frankenphp" {
		t.Errorf("site.Runtime = %q, want frankenphp", site.Runtime)
	}
}
