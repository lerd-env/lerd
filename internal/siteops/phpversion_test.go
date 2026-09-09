package siteops

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
)

// phpVersionTestSite registers an unsecured site in a temp registry and returns
// a copy callers can hand to SetSitePHPVersion. tweak adjusts the runtime shape
// before registration.
func phpVersionTestSite(t *testing.T, tweak func(*config.Site)) *config.Site {
	t.Helper()
	tmp := t.TempDir()
	// HOME too, not just XDG: the launchd/systemd unit for the fpm service is
	// written under os.UserHomeDir() (~/Library/LaunchAgents on macOS), which
	// reads $HOME rather than the XDG dirs. Without this the version switch
	// clobbers the real service files with a fake-podman path from this temp
	// tree, breaking the developer's own lerd install once the tree is cleaned up.
	t.Setenv("HOME", tmp)
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
	origRange := phpConstraintsFor
	origGap := imageGapFn
	t.Cleanup(func() {
		nginxReloadFn = origReload
		phpConstraintsFor = origRange
		imageGapFn = origGap
	})
	nginxReloadFn = func() error { reloads++; return nil }
	phpConstraintsFor = func(*config.Site) []string { return compactConstraints(phpRangeConstraint(min, max)) }
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

	if res.Version != "8.2" {
		t.Errorf("result = %+v, want version 8.2", res)
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

// The framework's supported range wins over the request, and a request it
// refuses changes nothing at all: the registry, the .php-version pin and the
// project's committed .lerd.yaml are all left as they were, so the user is told
// what they cannot have rather than handed something they did not ask for.
func TestSetSitePHPVersion_refusesOutsideFrameworkRange(t *testing.T) {
	site := phpVersionTestSite(t, asFPM)
	stubPHPVersionDeps(t, "8.3", "8.5")

	_, err := SetSitePHPVersion(site, "8.1", PHPVersionOpts{})
	var rangeErr *PHPRangeError
	if !errors.As(err, &rangeErr) {
		t.Fatalf("err = %v, want a PHPRangeError", err)
	}
	if rangeErr.Requested != "8.1" || len(rangeErr.Constraints) == 0 {
		t.Errorf("error = %+v, want the request and what it had to satisfy", rangeErr)
	}
	if _, err := os.Stat(filepath.Join(site.Path, ".php-version")); !os.IsNotExist(err) {
		t.Error("a refused pin wrote .php-version")
	}
	if site.PHPVersion != "8.4" {
		t.Errorf("site.PHPVersion = %q, want the original 8.4", site.PHPVersion)
	}
	stored, err := config.FindSite("app")
	if err != nil {
		t.Fatal(err)
	}
	if stored.PHPVersion != "8.4" {
		t.Errorf("registry = %q, want the original 8.4", stored.PHPVersion)
	}
}

// Forcing is the escape hatch, and it applies exactly what was asked for.
func TestSetSitePHPVersion_forcePinsOutsideRange(t *testing.T) {
	site := phpVersionTestSite(t, asFPM)
	stubPHPVersionDeps(t, "8.3", "8.5")

	res, err := SetSitePHPVersion(site, "8.1", PHPVersionOpts{Force: true})
	if err != nil {
		t.Fatalf("SetSitePHPVersion: %v", err)
	}
	if res.Version != "8.1" {
		t.Errorf("version = %q, want the forced 8.1", res.Version)
	}
	if got := readPHPVersionFile(t, site.Path); got != "8.1" {
		t.Errorf(".php-version = %q, want 8.1", got)
	}
}

// Input like "php8.2" must reduce to "8.2" before anything is derived from it;
// stored raw it produces image names like lerd-phpphp82-fpm-base (#1173).
func TestSetSitePHPVersion_normalizesPrefixedInput(t *testing.T) {
	site := phpVersionTestSite(t, asFPM)
	stubPHPVersionDeps(t, "", "")

	res, err := SetSitePHPVersion(site, "php8.2", PHPVersionOpts{})
	if err != nil {
		t.Fatalf("SetSitePHPVersion: %v", err)
	}
	if res.Version != "8.2" {
		t.Errorf("res.Version = %q, want 8.2", res.Version)
	}
	if got := readPHPVersionFile(t, site.Path); got != "8.2" {
		t.Errorf(".php-version = %q, want 8.2", got)
	}
	if site.PHPVersion != "8.2" {
		t.Errorf("site.PHPVersion = %q, want 8.2", site.PHPVersion)
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

// A worktree switch generates a vhost pointing at the target version's FPM
// container, so it has to write that version's quadlet and notify open
// dashboards exactly like the site path does. Without it the vhost can point at
// a container that was never created on this machine.
func TestSetSitePHPVersion_worktreeDoesTheSameRuntimeSetup(t *testing.T) {
	site := phpVersionTestSite(t, asFPM)
	stubPHPVersionDeps(t, "", "")

	wtPath := filepath.Join(site.Path, "wt", "feature")
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatal(err)
	}

	origDetect := detectWorktreesFn
	origDR := podman.DaemonReloadFn
	origNotify := podman.AfterUnitChange
	t.Cleanup(func() {
		detectWorktreesFn = origDetect
		podman.DaemonReloadFn = origDR
		podman.AfterUnitChange = origNotify
	})

	detectWorktreesFn = func(string, string) ([]gitpkg.Worktree, error) {
		return []gitpkg.Worktree{{Name: "feature", Branch: "feature", Path: wtPath, Domain: "feature.app.test"}}, nil
	}
	reloaded := 0
	podman.DaemonReloadFn = func() error { reloaded++; return nil }
	var notified []string
	podman.AfterUnitChange = func(name string) { notified = append(notified, name) }

	if _, err := SetSitePHPVersion(site, "8.3", PHPVersionOpts{Branch: "feature"}); err != nil {
		t.Fatalf("SetSitePHPVersion: %v", err)
	}

	if reloaded == 0 {
		t.Error("worktree switch did not write the target version's FPM quadlet")
	}
	if !slices.Contains(notified, "site:app") {
		t.Errorf("AfterUnitChange notifications = %v, want one for site:app", notified)
	}

	// The parent site keeps its own version; the override travels with the branch.
	stored, err := config.FindSite("app")
	if err != nil {
		t.Fatal(err)
	}
	if stored.PHPVersion != "8.4" {
		t.Errorf("parent site PHPVersion = %q, want 8.4 (unchanged)", stored.PHPVersion)
	}
	if got := config.WorktreePHPVersion(wtPath, stored.PHPVersion); got != "8.3" {
		t.Errorf("worktree version = %q, want 8.3", got)
	}
}

// The reported case. Definitions exist for Laravel 10 and up, so an older
// project borrows one and is marked guessed. Clamping to a borrowed range
// refuses the version the project actually requires: `lerd isolate 7.4` on a
// Laravel 8 app answered "7.4 isn't usable here" and moved it to 8.5.
// `lerd link` already declined to clamp a guessed definition; this path did not.
func TestPHPConstraintFor_GuessedFrameworkUsesComposer(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"),
		[]byte(`{"require":{"php":"^7.3|^8.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := getFrameworkFn
	t.Cleanup(func() { getFrameworkFn = orig })
	getFrameworkFn = func(string, string) (*config.Framework, bool) {
		return &config.Framework{
			Name: "laravel", Version: "10", VersionGuessed: true, DetectedVersion: "8",
			PHP: config.FrameworkPHP{Min: "8.1", Max: "8.3"},
		}, true
	}

	site := &config.Site{Name: "legacy", Path: dir, Framework: "laravel"}
	got := phpConstraintsFor(site)
	if !reflect.DeepEqual(got, []string{"^7.3|^8.0"}) {
		t.Errorf("constraints = %v, want the project's own ^7.3|^8.0", got)
	}
	if !php.SatisfiesAll("7.4", got...) {
		t.Error("7.4 was refused on a project that requires it")
	}
}

// The real definition still governs, so a modern Laravel keeps refusing a PHP
// it cannot run.
func TestPHPConstraintFor_RealFrameworkStillGoverns(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"),
		[]byte(`{"require":{"php":"^7.3|^8.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := getFrameworkFn
	t.Cleanup(func() { getFrameworkFn = orig })
	getFrameworkFn = func(string, string) (*config.Framework, bool) {
		return &config.Framework{
			Name: "laravel", Version: "13",
			PHP: config.FrameworkPHP{Min: "8.3", Max: "8.5"},
		}, true
	}

	site := &config.Site{Name: "modern", Path: dir, Framework: "laravel"}
	if got := phpConstraintsFor(site); !reflect.DeepEqual(got, []string{">=8.3 <=8.5", "^7.3|^8.0"}) {
		t.Errorf("constraints = %v, want the definition's range and the project's own", got)
	}
}

// No framework recognised at all: the project still gets a say.
func TestPHPConstraintFor_NoFrameworkUsesComposer(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"),
		[]byte(`{"require":{"php":"^8.2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	site := &config.Site{Name: "plain", Path: dir}
	if got := phpConstraintsFor(site); !reflect.DeepEqual(got, []string{"^8.2"}) {
		t.Errorf("constraints = %v, want ^8.2", got)
	}
}

// A definition can be the real one for the framework and still be wrong for the
// project in front of it: a Winter CMS install is detected as Laravel and served
// the Laravel 9 definition, capped at 8.2, while its own composer.json requires
// more than that. Nothing can satisfy both, so the project wins, since it is the
// one that has to boot.
func TestPHPConstraintsFor_ProjectWinsWhenRangeCannotServeIt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"),
		[]byte(`{"require":{"php":">=8.4"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := getFrameworkFn
	t.Cleanup(func() { getFrameworkFn = orig })
	getFrameworkFn = func(string, string) (*config.Framework, bool) {
		return &config.Framework{
			Name: "laravel", Version: "9",
			PHP: config.FrameworkPHP{Min: "8.0", Max: "8.2"},
		}, true
	}

	site := &config.Site{Name: "winter", Path: dir, Framework: "laravel"}
	if got := phpConstraintsFor(site); !reflect.DeepEqual(got, []string{">=8.4"}) {
		t.Errorf("constraints = %v, want the project's own >=8.4", got)
	}
}
