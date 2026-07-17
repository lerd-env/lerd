package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeGlobalConfig points the config dir at a temp tree and seeds config.yaml.
func writeGlobalConfig(t *testing.T, body string) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	invalidateGlobalCache()
	t.Cleanup(invalidateGlobalCache)

	path := GlobalConfigFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Extensions and packages belong to the user, not to a PHP version. A config
// written by an older lerd keys them per version, which is what makes a site
// silently lose chromium when it moves to a version nobody added it to. The
// migration folds every version's entries into one declared set.
func TestLoadGlobal_migratesPerVersionSetsToUnifiedSet(t *testing.T) {
	path := writeGlobalConfig(t, `
php:
  default_version: "8.4"
  extensions:
    "8.4": [mongodb, imagick]
    "8.5": [mongodb, xlswriter]
  packages:
    "8.4": [chromium]
    "8.5": [chromium, jq]
`)

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}

	wantExts := []string{"imagick", "mongodb", "xlswriter"}
	if !reflect.DeepEqual(cfg.PHP.Extensions, wantExts) {
		t.Errorf("extensions = %v, want the union %v", cfg.PHP.Extensions, wantExts)
	}
	wantPkgs := []string{"chromium", "jq"}
	if !reflect.DeepEqual(cfg.PHP.Packages, wantPkgs) {
		t.Errorf("packages = %v, want the union %v", cfg.PHP.Packages, wantPkgs)
	}

	// Folding must not write: LoadGlobal is a hot read path and a write there
	// races every other process loading the same config.
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	invalidateGlobalCache()
	if _, err := LoadGlobal(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("LoadGlobal wrote the config while folding; folding must stay read-only")
	}

	// The unified shape reaches disk on the next save any mutation performs,
	// and the legacy keys do not survive it.
	cfg.AddPackage("jq")
	if err := SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "8.4:") || strings.Contains(string(raw), `"8.4":`) {
		t.Errorf("per-version keys survived a save after migration:\n%s", raw)
	}
}

// A config already on the unified shape must load untouched and must not be
// rewritten: a no-op load that saves would churn mtime on every command.
func TestLoadGlobal_leavesUnifiedSetAlone(t *testing.T) {
	path := writeGlobalConfig(t, `
php:
  default_version: "8.4"
  extensions: [mongodb, imagick]
  packages: [chromium]
`)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}

	if !reflect.DeepEqual(cfg.PHP.Extensions, []string{"mongodb", "imagick"}) {
		t.Errorf("extensions = %v, want the declared order preserved", cfg.PHP.Extensions)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("an already-migrated config was rewritten on load")
	}
}

// Loading twice must be stable: the second pass has nothing left to fold.
func TestLoadGlobal_migrationIsIdempotent(t *testing.T) {
	writeGlobalConfig(t, `
php:
  extensions:
    "8.4": [mongodb]
  packages:
    "8.4": [chromium]
`)

	first, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	invalidateGlobalCache()
	second, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal (second): %v", err)
	}

	if !reflect.DeepEqual(first.PHP.Extensions, second.PHP.Extensions) {
		t.Errorf("extensions drifted across loads: %v then %v", first.PHP.Extensions, second.PHP.Extensions)
	}
	if !reflect.DeepEqual(first.PHP.Packages, second.PHP.Packages) {
		t.Errorf("packages drifted across loads: %v then %v", first.PHP.Packages, second.PHP.Packages)
	}
}

// An empty per-version map is not a migration: it must not mark the config
// dirty and trigger a pointless save.
func TestLoadGlobal_emptyPerVersionMapIsNotAMigration(t *testing.T) {
	path := writeGlobalConfig(t, `
php:
  default_version: "8.4"
`)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := LoadGlobal(); err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("a config with no custom sets was rewritten on load")
	}
}

// An image build takes minutes and records what it realised part-way through.
// A command that loaded the config before that build and saved its own copy
// afterwards would erase the record, so mutations re-read first.
func TestUpdateGlobal_doesNotClobberAConcurrentWrite(t *testing.T) {
	writeGlobalConfig(t, `
php:
  extensions: [mongodb]
`)

	// Stand in for RecordRealisedSet, which does its own load/save from inside
	// the build that a command is waiting on.
	if err := UpdateGlobal(func(c *GlobalConfig) {
		c.SetRealised("8.4", RealisedPHPSet{Extensions: []string{"mongodb"}})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}

	// The command now reverts its extension. It must not take the realised
	// record with it.
	if err := UpdateGlobal(func(c *GlobalConfig) { c.RemoveExtension("mongodb") }); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}

	got, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetExtensions()) != 0 {
		t.Errorf("extensions = %v, want the removal applied", got.GetExtensions())
	}
	if r := got.GetRealised("8.4"); !reflect.DeepEqual(r.Extensions, []string{"mongodb"}) {
		t.Errorf("realised record was clobbered by a later mutation: %+v", r)
	}
}

func TestPHPSetAccessors(t *testing.T) {
	cfg := defaultConfig()

	cfg.AddExtension("mongodb")
	cfg.AddExtension("mongodb") // duplicate must be a no-op
	cfg.AddExtension("imagick")
	if got := cfg.GetExtensions(); !reflect.DeepEqual(got, []string{"mongodb", "imagick"}) {
		t.Errorf("GetExtensions = %v, want [mongodb imagick]", got)
	}

	cfg.SetExtApkDeps("mongodb", []string{"openssl-dev"})
	cfg.RemoveExtension("mongodb")
	if got := cfg.GetExtensions(); !reflect.DeepEqual(got, []string{"imagick"}) {
		t.Errorf("after remove, GetExtensions = %v, want [imagick]", got)
	}
	// The deps of an extension nobody declares any more are dead weight.
	if deps := cfg.AllExtApkDeps()["mongodb"]; deps != nil {
		t.Errorf("apk deps survived the removal of their extension: %v", deps)
	}

	cfg.AddPackage("chromium")
	cfg.AddPackage("chromium")
	if got := cfg.GetPackages(); !reflect.DeepEqual(got, []string{"chromium"}) {
		t.Errorf("GetPackages = %v, want [chromium]", got)
	}
	cfg.RemovePackage("chromium")
	if got := cfg.GetPackages(); len(got) != 0 {
		t.Errorf("GetPackages = %v, want empty", got)
	}
}

// The realised set records what a version's image actually loaded, so lerd can
// warn about a gap instead of advertising what the image does not have.
func TestRealisedSetRoundTrips(t *testing.T) {
	cfg := defaultConfig()

	cfg.SetRealised("7.4", RealisedPHPSet{Extensions: []string{"imagick"}, Packages: []string{"jq"}})
	cfg.SetRealised("8.5", RealisedPHPSet{Extensions: []string{"imagick", "mongodb"}, Packages: []string{"jq", "chromium"}})

	got := cfg.GetRealised("7.4")
	if !reflect.DeepEqual(got.Extensions, []string{"imagick"}) {
		t.Errorf("realised 7.4 extensions = %v, want [imagick]", got.Extensions)
	}
	// mongodb does not build on 7.4, so it is declared but not realised there.
	if missing := cfg.MissingFromImage("7.4", []string{"imagick", "mongodb"}); !reflect.DeepEqual(missing, []string{"mongodb"}) {
		t.Errorf("MissingFromImage(7.4) = %v, want [mongodb]", missing)
	}
	if missing := cfg.MissingFromImage("8.5", []string{"imagick", "mongodb"}); len(missing) != 0 {
		t.Errorf("MissingFromImage(8.5) = %v, want none", missing)
	}
	// A version with no recorded build must not be reported as missing
	// everything: nothing is known about it yet.
	if missing := cfg.MissingFromImage("8.3", []string{"imagick"}); len(missing) != 0 {
		t.Errorf("MissingFromImage on an unbuilt version = %v, want none", missing)
	}
}
