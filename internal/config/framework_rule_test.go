package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// The store index is JSON with snake_case keys. FrameworkRule needs json tags,
// not just yaml, or "composer_sections" and "version_key" silently fail to bind
// (Go's case-insensitive fallback does not cross underscores), which drops
// Symfony's flex-require / extra.symfony.require version detection.
func TestFrameworkRule_JSONSnakeCaseTags(t *testing.T) {
	data := []byte(`{"composer":"symfony/framework-bundle","composer_sections":["flex-require"],"version_key":"extra.symfony.require"}`)
	var r FrameworkRule
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.VersionKey != "extra.symfony.require" {
		t.Errorf("version_key not bound: %q", r.VersionKey)
	}
	if len(r.ComposerSections) != 1 || r.ComposerSections[0] != "flex-require" {
		t.Errorf("composer_sections not bound: %+v", r.ComposerSections)
	}
}

// A bootstrap step belongs only on a project that has not been bootstrapped, so
// missing_file matches while its file is absent and stops matching once the
// step's own run has written it.
func TestMatchesRule_MissingFile(t *testing.T) {
	dir := t.TempDir()
	rule := FrameworkRule{MissingFile: "app/etc/config.php"}
	if !MatchesRule(dir, rule) {
		t.Error("missing_file should match while the file is absent")
	}
	if err := os.MkdirAll(filepath.Join(dir, "app", "etc"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(dir, "app", "etc", "config.php"), "<?php return [];\n")
	if MatchesRule(dir, rule) {
		t.Error("missing_file should not match once the file exists")
	}
}

func TestFrameworkRule_MissingFileYAML(t *testing.T) {
	src := []byte(`
name: magento
version: "2"
setup:
  - label: Install the store
    command: php bin/magento setup:install
    check:
      missing_file: app/etc/config.php
`)
	var fw Framework
	if err := yaml.Unmarshal(src, &fw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(fw.Setup) != 1 || fw.Setup[0].Check == nil {
		t.Fatalf("setup check not parsed: %+v", fw.Setup)
	}
	if fw.Setup[0].Check.MissingFile != "app/etc/config.php" {
		t.Errorf("missing_file not bound: %+v", fw.Setup[0].Check)
	}
}

// The store reaches installs running older binaries, which drop missing_file
// and see an empty rule. That rule must fail, hiding the step, rather than pass
// and offer a store-wiping install on a store that is already installed.
func TestMatchesRule_EmptyRuleNeverMatches(t *testing.T) {
	if MatchesRule(t.TempDir(), FrameworkRule{}) {
		t.Error("an empty rule must not match")
	}
}

// A framework whose project is installed by a command of its own declares it as
// an install block, which carries the file that command writes.
func TestFramework_InstallYAML(t *testing.T) {
	src := []byte(`
name: drupal
version: "11"
install:
  label: Install Drupal
  command: php vendor/bin/drush site:install --yes
  missing_file: web/sites/default/settings.php
`)
	var fw Framework
	if err := yaml.Unmarshal(src, &fw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if fw.Install == nil {
		t.Fatal("install block not bound")
	}
	if fw.Install.Label != "Install Drupal" || fw.Install.MissingFile != "web/sites/default/settings.php" {
		t.Errorf("install block bound wrong: %+v", fw.Install)
	}
}
