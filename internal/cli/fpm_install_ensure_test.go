package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The default version comes first and every active site adds its own once, so
// install brings up exactly the runtimes something on this machine points at.
func TestFPMVersionsToEnsure_defaultFirstThenEachActiveSiteOnce(t *testing.T) {
	sites := []config.Site{
		{Name: "shop", PHPVersion: "8.4"},
		{Name: "blog", PHPVersion: "8.3"},
		{Name: "api", PHPVersion: "8.4"},
		{Name: "inherits", PHPVersion: ""},
	}

	got := fpmVersionsToEnsure("8.3", sites)

	if want := []string{"8.3", "8.4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("versions = %v, want %v", got, want)
	}
}

// A paused or ignored site is not served, so its version is not worth a build.
func TestFPMVersionsToEnsure_skipsPausedAndIgnoredSites(t *testing.T) {
	sites := []config.Site{
		{Name: "paused", PHPVersion: "8.2", Paused: true},
		{Name: "ignored", PHPVersion: "8.1", Ignored: true},
		{Name: "live", PHPVersion: "8.5"},
	}

	got := fpmVersionsToEnsure("8.4", sites)

	if want := []string{"8.4", "8.5"}; !reflect.DeepEqual(got, want) {
		t.Errorf("versions = %v, want %v", got, want)
	}
}

// No default configured yet (a fresh install) must not queue an empty version.
func TestFPMVersionsToEnsure_dropsEmptyVersions(t *testing.T) {
	got := fpmVersionsToEnsure("", []config.Site{{Name: "orphan", PHPVersion: ""}})

	if len(got) != 0 {
		t.Errorf("versions = %v, want none", got)
	}
}

// Only a version with something to build gets the progress loader. The rest are
// no-ops whose podman output would otherwise land raw in the install log.
func TestFPMEnsurePlan_splitsOnWhatActuallyBuilds(t *testing.T) {
	orig := fpmImageCurrentFn
	t.Cleanup(func() { fpmImageCurrentFn = orig })
	fpmImageCurrentFn = func(v string) bool { return v == "8.3" }

	build, quiet := fpmEnsurePlan([]string{"8.3", "8.4", "8.5"})

	if want := []string{"8.4", "8.5"}; !reflect.DeepEqual(build, want) {
		t.Errorf("build = %v, want %v", build, want)
	}
	if want := []string{"8.3"}; !reflect.DeepEqual(quiet, want) {
		t.Errorf("quiet = %v, want %v", quiet, want)
	}
}

func TestMergeVersions_keepsBaseOrderAndAddsMissing(t *testing.T) {
	got := mergeVersions([]string{"8.5", "8.4"}, []string{"8.4", "8.3", ""})
	if want := []string{"8.5", "8.4", "8.3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("merged = %v, want %v", got, want)
	}
}

// The reported case: a fresh install on a machine that already holds projects in
// the default parked directory. The registry is still empty at install time, so
// the version those projects pin has to come from reading them, or the watcher
// registers a site minutes later against an image nothing built.
func TestParkedFPMVersions_findsTheVersionAParkedProjectPins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	parked := filepath.Join(home, "Lerd")
	project := filepath.Join(parked, "legacy")
	if err := os.MkdirAll(filepath.Join(project, "public"), 0755); err != nil {
		t.Fatal(err)
	}
	// composer.json is what parkAdmits accepts, .php-version is the pin.
	if err := os.WriteFile(filepath.Join(project, "composer.json"), []byte(`{"name":"acme/legacy"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".php-version"), []byte("8.3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.GlobalConfig{ParkedDirectories: []string{parked}}
	cfg.PHP.DefaultVersion = "8.5"

	got := parkedFPMVersions(cfg)
	if !slices.Contains(got, "8.3") {
		t.Errorf("versions = %v, want the parked project's 8.3 pin", got)
	}
}

func TestParkedFPMVersions_ignoresADirectoryThatIsNotAProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	parked := filepath.Join(home, "Lerd")
	if err := os.MkdirAll(filepath.Join(parked, "notes"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.GlobalConfig{ParkedDirectories: []string{parked}}

	if got := parkedFPMVersions(cfg); len(got) != 0 {
		t.Errorf("versions = %v, want none for a directory that is not a PHP project", got)
	}
}

func TestParkedFPMVersions_toleratesAMissingParkedDirectory(t *testing.T) {
	cfg := &config.GlobalConfig{ParkedDirectories: []string{filepath.Join(t.TempDir(), "nope")}}
	if got := parkedFPMVersions(cfg); len(got) != 0 {
		t.Errorf("versions = %v, want none", got)
	}
	if got := parkedFPMVersions(nil); got != nil {
		t.Errorf("versions = %v, want nil for no config", got)
	}
}

// The wiring, not just the helper: ensuredFPMVersions is what install builds
// from, so a parked project's pin has to reach it. Asserting on the helper alone
// passes with the two halves unconnected, which is the whole defect.
func TestEnsuredFPMVersions_includesAParkedProjectsPin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	parked := filepath.Join(home, "Lerd")
	project := filepath.Join(parked, "legacy")
	if err := os.MkdirAll(filepath.Join(project, "public"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "composer.json"), []byte(`{"name":"acme/legacy"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".php-version"), []byte("8.3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.PHP.DefaultVersion = "8.5"
	cfg.ParkedDirectories = []string{parked}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}

	got := ensuredFPMVersions()
	if !slices.Contains(got, "8.5") {
		t.Errorf("versions = %v, want the default 8.5", got)
	}
	if !slices.Contains(got, "8.3") {
		t.Errorf("versions = %v, want the parked project's 8.3, which install would otherwise not build", got)
	}
}
