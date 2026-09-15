package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeStoreFramework drops a versioned store definition on disk the way the
// store fetch would, so detection sees a framework shipping one file per major.
func writeStoreFramework(t *testing.T, name, version, body string) {
	t.Helper()
	dir := StoreFrameworksDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir store: %v", err)
	}
	path := filepath.Join(dir, name+"@"+version+".yaml")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// A framework ships one definition per major and the majors can detect on
// entirely different evidence. CodeIgniter 3 keys off system/core/CodeIgniter.php
// while CodeIgniter 4 keys off spark, so a v4 project matches nothing the v3
// file declares. Detection must read every major's rules, not just the first
// file the glob returns.
func TestDetectFramework_LaterMajorWithItsOwnRules(t *testing.T) {
	setConfigDir(t)
	writeStoreFramework(t, "codeigniter", "3", `
name: codeigniter
version: "3"
detect:
  - file: system/core/CodeIgniter.php
  - composer: codeigniter/framework
`)
	writeStoreFramework(t, "codeigniter", "4", `
name: codeigniter
version: "4"
detect:
  - file: spark
  - composer: codeigniter4/framework
`)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "spark"), []byte("#!/usr/bin/env php"), 0644) //nolint:errcheck

	name, ok := DetectFramework(dir)
	if !ok {
		t.Fatal("expected the v4 project to be detected")
	}
	if name != "codeigniter" {
		t.Errorf("expected codeigniter, got %q", name)
	}
}

// The earlier major must keep working once the later one is consulted too.
func TestDetectFramework_EarlierMajorStillMatches(t *testing.T) {
	setConfigDir(t)
	writeStoreFramework(t, "codeigniter", "3", `
name: codeigniter
version: "3"
detect:
  - file: system/core/CodeIgniter.php
`)
	writeStoreFramework(t, "codeigniter", "4", `
name: codeigniter
version: "4"
detect:
  - file: spark
`)

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "system", "core"), 0755)                                      //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "system", "core", "CodeIgniter.php"), []byte("<?php"), 0644) //nolint:errcheck

	name, ok := DetectFramework(dir)
	if !ok {
		t.Fatal("expected the v3 project to be detected")
	}
	if name != "codeigniter" {
		t.Errorf("expected codeigniter, got %q", name)
	}
}

// frameworkDetectRules feeds DetectMajorVersion, which walks the composer rules
// looking for the one the project actually locked. With only the first file's
// rules it can never tell two majors apart that name different packages.
func TestFrameworkDetectRules_UnionsEveryMajor(t *testing.T) {
	setConfigDir(t)
	writeStoreFramework(t, "codeigniter", "3", `
name: codeigniter
version: "3"
detect:
  - file: system/core/CodeIgniter.php
  - composer: codeigniter/framework
`)
	writeStoreFramework(t, "codeigniter", "4", `
name: codeigniter
version: "4"
detect:
  - file: spark
  - composer: codeigniter4/framework
`)

	rules := frameworkDetectRules("codeigniter")
	var composer []string
	for _, r := range rules {
		if r.Composer != "" {
			composer = append(composer, r.Composer)
		}
	}
	want := map[string]bool{"codeigniter/framework": true, "codeigniter4/framework": true}
	for _, c := range composer {
		delete(want, c)
	}
	if len(want) != 0 {
		t.Errorf("missing composer rules %v, got %v", want, composer)
	}
}

// Rules must not repeat when several majors declare the same evidence, or
// DetectMajorVersion does the same lookup once per file.
func TestFrameworkDetectRules_Deduplicates(t *testing.T) {
	setConfigDir(t)
	for _, v := range []string{"10", "11", "12"} {
		writeStoreFramework(t, "laravel", v, `
name: laravel
version: "`+v+`"
detect:
  - file: artisan
  - composer: laravel/framework
`)
	}

	rules := frameworkDetectRules("laravel")
	if len(rules) != 2 {
		t.Errorf("expected 2 deduplicated rules, got %d: %+v", len(rules), rules)
	}
}

// A framework whose skeleton is the framework itself never requires its own
// package, so the major has to come off a source constant. CodeIgniter 3 is
// the case in the store; without this it resolves to nothing and the site is
// labelled with whatever major happens to be newest.
func TestDetectMajorVersion_FromVersionFile(t *testing.T) {
	setConfigDir(t)
	writeStoreFramework(t, "codeigniter", "3", `
name: codeigniter
version: "3"
detect:
  - file: system/core/CodeIgniter.php
    version_file: system/core/CodeIgniter.php
    version_pattern: "CI_VERSION\\s*=\\s*'([^']+)'"
  - composer: codeigniter/framework
`)
	writeStoreFramework(t, "codeigniter", "4", `
name: codeigniter
version: "4"
detect:
  - file: spark
  - composer: codeigniter4/framework
`)

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "system", "core"), 0755) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "system", "core", "CodeIgniter.php"),
		[]byte("<?php\n\tconst CI_VERSION = '3.1.13';\n"), 0644) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "composer.json"),
		[]byte(`{"name":"codeigniter/framework","require":{"php":">=5.3.7"}}`), 0644) //nolint:errcheck

	if got := DetectMajorVersion(dir, "codeigniter"); got != "3" {
		t.Errorf("expected major 3, got %q", got)
	}
}

// The v4 appstarter does require its framework package, so the lock/manifest
// answers first and the v3 source constant must not shadow it.
func TestDetectMajorVersion_ComposerBeatsVersionFile(t *testing.T) {
	setConfigDir(t)
	writeStoreFramework(t, "codeigniter", "3", `
name: codeigniter
version: "3"
detect:
  - file: system/core/CodeIgniter.php
    version_file: system/core/CodeIgniter.php
    version_pattern: "CI_VERSION\\s*=\\s*'([^']+)'"
  - composer: codeigniter/framework
`)
	writeStoreFramework(t, "codeigniter", "4", `
name: codeigniter
version: "4"
detect:
  - file: spark
  - composer: codeigniter4/framework
`)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "spark"), []byte("#!/usr/bin/env php"), 0644) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "composer.json"),
		[]byte(`{"name":"codeigniter4/appstarter","require":{"codeigniter4/framework":"^4.7"}}`), 0644) //nolint:errcheck

	if got := DetectMajorVersion(dir, "codeigniter"); got != "4" {
		t.Errorf("expected major 4, got %q", got)
	}
}
