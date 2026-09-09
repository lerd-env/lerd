package phpsets

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// On the native runtime there is no image to measure against, and reporting
// nothing built sent the extensions tab to "PHP 8.5 has no image yet. Run
// 'lerd php:rebuild 8.5'", a command that refuses under that runtime. What can
// be measured is the binary, which reports its own compiled-in set.
func TestVersionStatusReadsTheHostBuildUnderNative(t *testing.T) {
	prevNative, prevBuilt, prevMods := nativeRuntimeFn, nativeBuiltFn, nativeModulesFn
	nativeRuntimeFn = func() bool { return true }
	nativeBuiltFn = func(string) bool { return true }
	nativeModulesFn = func(string) ([]string, error) {
		return []string{"intl", "redis", "Zend OPcache"}, nil
	}
	t.Cleanup(func() { nativeRuntimeFn, nativeBuiltFn, nativeModulesFn = prevNative, prevBuilt, prevMods })

	cfg := &config.GlobalConfig{}
	cfg.PHP.Extensions = []string{"intl", "opcache", "imagick"}

	r := VersionStatus(cfg, "8.5")
	if !r.Built {
		t.Fatal("a version with a host build installed is built")
	}
	if r.NeedsRebuild {
		t.Error("there is no image to rebuild on the native runtime")
	}
	has := strings.Join(r.Extensions.Has, ",")
	// opcache is reported by PHP as "Zend OPcache", so a literal comparison
	// would call a compiled-in extension missing.
	if !strings.Contains(has, "intl") || !strings.Contains(has, "opcache") {
		t.Errorf("Has = %v, want the compiled-in ones", r.Extensions.Has)
	}
	if strings.Contains(has, "imagick") {
		t.Errorf("Has = %v, imagick is not in this build", r.Extensions.Has)
	}
	if len(r.Extensions.Cannot) != 1 || r.Extensions.Cannot[0] != "imagick" {
		t.Errorf("Cannot = %v, want imagick", r.Extensions.Cannot)
	}
}

// Nothing installed for that version is the one case where the native runtime
// really has nothing to report.
func TestVersionStatusUnbuiltUnderNative(t *testing.T) {
	prevNative, prevBuilt := nativeRuntimeFn, nativeBuiltFn
	nativeRuntimeFn = func() bool { return true }
	nativeBuiltFn = func(string) bool { return false }
	t.Cleanup(func() { nativeRuntimeFn, nativeBuiltFn = prevNative, prevBuilt })

	if VersionStatus(&config.GlobalConfig{}, "8.6").Built {
		t.Error("a version with no host build is not built")
	}
}

// The tab reads php -m for the version. Under the native runtime that comes
// from the binary; keyed on an image ID before this, which is empty there, so
// a build carrying sixty extensions reported none.
func TestModulesReadsTheHostBuildUnderNative(t *testing.T) {
	prevNative, prevBuilt, prevMods := nativeRuntimeFn, nativeBuiltFn, nativeModulesFn
	nativeRuntimeFn = func() bool { return true }
	nativeBuiltFn = func(string) bool { return true }
	nativeModulesFn = func(string) ([]string, error) {
		return []string{"[PHP Modules]", "intl", "Zend OPcache", "redis", "", "[Zend Modules]", "Zend OPcache"}, nil
	}
	t.Cleanup(func() { nativeRuntimeFn, nativeBuiltFn, nativeModulesFn = prevNative, prevBuilt, prevMods })

	mods, err := Modules("8.5")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(mods, ",")
	// Folded to the names people install, deduped across both sections.
	if got != "intl,opcache,redis" {
		t.Errorf("Modules = %q, want intl,opcache,redis", got)
	}
}

func TestModulesEmptyWhenNoHostBuild(t *testing.T) {
	prevNative, prevBuilt := nativeRuntimeFn, nativeBuiltFn
	nativeRuntimeFn = func() bool { return true }
	nativeBuiltFn = func(string) bool { return false }
	t.Cleanup(func() { nativeRuntimeFn, nativeBuiltFn = prevNative, prevBuilt })

	if mods, err := Modules("8.6"); err != nil || len(mods) != 0 {
		t.Errorf("Modules = %v, %v; want none for a version with no build", mods, err)
	}
}
