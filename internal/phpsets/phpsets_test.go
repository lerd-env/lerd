package phpsets

import (
	"reflect"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func testConfig(t *testing.T) *config.GlobalConfig {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	return &config.GlobalConfig{}
}

// stubImages pins what each version's image is, so a test drives the report's
// decisions without podman.
func stubImages(t *testing.T, exists func(string) bool, stale func(string) bool) {
	t.Helper()
	origExists, origStale := imageExistsFn, imageStaleFn
	t.Cleanup(func() { imageExistsFn = origExists; imageStaleFn = origStale })
	imageExistsFn = exists
	imageStaleFn = stale
}

// The three states are the whole point: they are different problems with
// different answers, and a flat "declared" list advertises what an image does
// not have.
func TestVersionStatus_reportsTheThreeStates(t *testing.T) {
	cfg := testConfig(t)
	cfg.AddExtension("mongodb")
	cfg.AddPackage("chromium")
	cfg.SetRealised("8.5", config.RealisedPHPSet{Extensions: []string{"mongodb"}, Packages: []string{"chromium"}})
	// 7.4's Alpine 3.16 could not build mongodb, but did install chromium.
	cfg.SetRealised("7.4", config.RealisedPHPSet{Packages: []string{"chromium"}})

	stubImages(t, func(string) bool { return true }, func(v string) bool { return v == "8.1" })

	current := VersionStatus(cfg, "8.5")
	if !current.Built || current.NeedsRebuild {
		t.Errorf("8.5 = %+v, want a built, current image", current)
	}
	if !reflect.DeepEqual(current.Extensions.Has, []string{"mongodb"}) || len(current.Extensions.Cannot) != 0 {
		t.Errorf("8.5 extensions = %+v, want mongodb present", current.Extensions)
	}

	legacy := VersionStatus(cfg, "7.4")
	if !reflect.DeepEqual(legacy.Extensions.Cannot, []string{"mongodb"}) {
		t.Errorf("7.4 extensions = %+v, want mongodb reported as unbuildable", legacy.Extensions)
	}
	if !reflect.DeepEqual(legacy.Packages.Has, []string{"chromium"}) {
		t.Errorf("7.4 packages = %+v, want chromium present", legacy.Packages)
	}

	// A stale image predates the set, so nothing about it is claimed either way:
	// a rebuild, not a verdict, is what it needs.
	stale := VersionStatus(cfg, "8.1")
	if !stale.NeedsRebuild {
		t.Errorf("8.1 = %+v, want NeedsRebuild", stale)
	}
	if len(stale.Extensions.Has) != 0 || len(stale.Extensions.Cannot) != 0 {
		t.Errorf("a stale image must claim nothing, got %+v", stale.Extensions)
	}
}

// An unbuilt version is not evidence that anything is missing.
func TestVersionStatus_unbuiltVersionClaimsNothing(t *testing.T) {
	cfg := testConfig(t)
	cfg.AddExtension("mongodb")
	stubImages(t, func(string) bool { return false }, func(string) bool { return true })

	got := VersionStatus(cfg, "8.2")
	if got.Built || got.NeedsRebuild {
		t.Errorf("%+v, want an unbuilt version that claims nothing", got)
	}
	if len(got.Extensions.Has) != 0 || len(got.Extensions.Cannot) != 0 {
		t.Errorf("unbuilt image claimed something: %+v", got.Extensions)
	}
	// The declared set is still worth reporting: it is what the user asked for.
	if !reflect.DeepEqual(got.Extensions.Declared, []string{"mongodb"}) {
		t.Errorf("Declared = %v, want [mongodb]", got.Extensions.Declared)
	}
}

// php -m prints display names ("Zend OPcache", "PDO"), which are folded onto the
// names users install so the list matches what they typed.
func TestParseModules(t *testing.T) {
	got := parseModules("[PHP Modules]\nCore\nZend OPcache\nSimpleXML\nmongodb\n\n[Zend Modules]\nZend OPcache\n")

	want := []string{"core", "mongodb", "opcache", "simplexml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseModules = %v, want %v (canonicalised, sorted, deduped)", got, want)
	}
}

func TestParseModules_empty(t *testing.T) {
	if got := parseModules(""); len(got) != 0 {
		t.Errorf("parseModules(\"\") = %v, want empty", got)
	}
}

// Reading php -m starts a container, so it is cached against the image's ID:
// the same image can never report different modules, and a rebuilt one always
// gets a fresh read.
func TestModules_cachedByImageID(t *testing.T) {
	origID, origRead := imageIDFn, readModulesFn
	t.Cleanup(func() { imageIDFn = origID; readModulesFn = origRead; modulesCache.clear() })
	modulesCache.clear()

	id := "sha256:aaa"
	reads := 0
	imageIDFn = func(string) string { return id }
	readModulesFn = func(string) (string, error) {
		reads++
		return "[PHP Modules]\nmongodb\n", nil
	}

	for range 3 {
		if got, err := Modules("8.5"); err != nil || !reflect.DeepEqual(got, []string{"mongodb"}) {
			t.Fatalf("Modules = %v, %v", got, err)
		}
	}
	if reads != 1 {
		t.Errorf("php -m ran %d times for one image, want 1", reads)
	}

	// A rebuild changes the image ID, so the cache must not answer for it.
	id = "sha256:bbb"
	if _, err := Modules("8.5"); err != nil {
		t.Fatal(err)
	}
	if reads != 2 {
		t.Errorf("php -m ran %d times after a rebuild, want 2", reads)
	}
}

// A version with no image has no modules to report, and must not be cached as
// "none" for whenever it is eventually built.
func TestModules_unbuiltVersion(t *testing.T) {
	origID := imageIDFn
	t.Cleanup(func() { imageIDFn = origID; modulesCache.clear() })
	modulesCache.clear()
	imageIDFn = func(string) string { return "" }

	got, err := Modules("8.2")
	if err != nil {
		t.Fatalf("Modules on an unbuilt version errored: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Modules = %v, want empty for an unbuilt version", got)
	}
}

// An entry that landed nowhere is a broken build environment, not a version
// boundary, and the realised set already holds what tells those apart.
func TestNowhereBuilt(t *testing.T) {
	cfg := testConfig(t)
	cfg.AddExtension("mongodb")
	cfg.AddExtension("yaml")
	cfg.SetRealised("8.5", config.RealisedPHPSet{Extensions: []string{"mongodb"}})
	cfg.SetRealised("7.4", config.RealisedPHPSet{})
	stubImages(t, func(v string) bool { return v == "8.5" || v == "7.4" }, func(string) bool { return false })

	reports := StatusAll(cfg, []string{"8.5", "8.1", "7.4"})
	got := NowhereBuilt(reports, func(r Report) SetState { return r.Extensions })
	if !reflect.DeepEqual(got, []string{"yaml"}) {
		t.Errorf("NowhereBuilt = %v, want [yaml]: mongodb built on 8.5, yaml built nowhere", got)
	}
}

// With no current image anywhere there is nothing to conclude, so nothing is
// claimed: an unbuilt version is not evidence that an entry failed.
func TestNowhereBuilt_noJudgeableImage(t *testing.T) {
	cfg := testConfig(t)
	cfg.AddExtension("yaml")
	stubImages(t, func(string) bool { return true }, func(string) bool { return true })

	reports := StatusAll(cfg, []string{"8.5", "8.4"})
	if got := NowhereBuilt(reports, func(r Report) SetState { return r.Extensions }); got != nil {
		t.Errorf("NowhereBuilt = %v, want nil when every image is stale", got)
	}
}
