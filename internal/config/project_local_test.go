package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/feedback"
)

func writeLocalFixture(t *testing.T, dir, base, local string) {
	t.Helper()
	if base != "" {
		if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(base), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if local != "" {
		if err := os.WriteFile(filepath.Join(dir, ".lerd.local.yaml"), []byte(local), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadProjectConfig_localOverridesBase(t *testing.T) {
	dir := t.TempDir()
	writeLocalFixture(t, dir,
		"domains:\n  - acme\nphp_version: \"8.3\"\nsecured: true\n",
		"domains:\n  - acme-branch\ndb_isolated: true\n")

	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Domains) != 1 || cfg.Domains[0] != "acme-branch" {
		t.Errorf("domains = %v, want the local file's list", cfg.Domains)
	}
	if !cfg.DBIsolated {
		t.Error("db_isolated from the local file was not applied")
	}
	if cfg.PHPVersion != "8.3" {
		t.Errorf("php_version = %q, want the base value kept", cfg.PHPVersion)
	}
	if !cfg.Secured {
		t.Error("secured from the base file was dropped")
	}
}

func TestLoadProjectConfig_localAloneWithoutBase(t *testing.T) {
	dir := t.TempDir()
	writeLocalFixture(t, dir, "", "php_version: \"8.4\"\n")

	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.PHPVersion != "8.4" {
		t.Errorf("php_version = %q, want 8.4 from the local file alone", cfg.PHPVersion)
	}
}

func TestLoadProjectConfig_localParseErrorNamesTheFile(t *testing.T) {
	dir := t.TempDir()
	writeLocalFixture(t, dir, "php_version: \"8.3\"\n", "- domains\n  - acme\n")

	cfg, err := LoadProjectConfig(dir)
	if err == nil {
		t.Fatal("a top-level sequence in the local file must not parse")
	}
	if !strings.Contains(err.Error(), ".lerd.local.yaml") {
		t.Errorf("error = %q, want the local file named in it", err)
	}
	if cfg == nil || !cfg.IsEmpty() {
		t.Errorf("failed load = %+v, want an empty config", cfg)
	}
}

// The point of the local file: a save triggered by anything else (a worker
// starting, a runtime switch) must never bake the override into the committed
// file, and must not silently revert it either.
func TestSaveProjectConfig_keepsLocalKeysOutOfBaseFile(t *testing.T) {
	dir := t.TempDir()
	writeLocalFixture(t, dir,
		"domains:\n  - acme\nphp_version: \"8.3\"\n",
		"domains:\n  - acme-branch\ndb_isolated: true\n")

	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg.Workers = []string{"queue"}
	if err := SaveProjectConfig(dir, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".lerd.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "acme-branch") {
		t.Errorf(".lerd.yaml got the local domain written into it:\n%s", data)
	}
	if strings.Contains(string(data), "db_isolated") {
		t.Errorf(".lerd.yaml got the local db_isolated written into it:\n%s", data)
	}
	if !strings.Contains(string(data), "- acme\n") {
		t.Errorf(".lerd.yaml lost its own domain:\n%s", data)
	}
	if !strings.Contains(string(data), "queue") {
		t.Errorf("the actual change was not saved:\n%s", data)
	}

	// The merged view is unchanged by the round trip.
	after, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(after.Domains) != 1 || after.Domains[0] != "acme-branch" || !after.DBIsolated {
		t.Errorf("after save the override is gone: domains=%v db_isolated=%v", after.Domains, after.DBIsolated)
	}
}

// A key the local file sets but the base file never had must not appear in the
// base file after a save, not even as an empty value.
func TestSaveProjectConfig_dropsLocalOnlyKeys(t *testing.T) {
	dir := t.TempDir()
	writeLocalFixture(t, dir, "php_version: \"8.3\"\n", "runtime: frankenphp\n")

	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := SaveProjectConfig(dir, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".lerd.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "runtime") {
		t.Errorf("local-only key leaked into .lerd.yaml:\n%s", data)
	}
}

// The cache keys on both files, so editing only the local one is picked up.
func TestLoadProjectConfig_cacheTracksLocalFile(t *testing.T) {
	dir := t.TempDir()
	writeLocalFixture(t, dir, "php_version: \"8.3\"\n", "")
	if cfg, _ := LoadProjectConfig(dir); cfg.PHPVersion != "8.3" {
		t.Fatalf("first load = %q", cfg.PHPVersion)
	}

	writeLocalFixture(t, dir, "", "php_version: \"8.4\"\n")
	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.PHPVersion != "8.4" {
		t.Errorf("php_version = %q, want the newly written local file to win", cfg.PHPVersion)
	}

	if err := os.Remove(filepath.Join(dir, ".lerd.local.yaml")); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := LoadProjectConfig(dir); cfg.PHPVersion != "8.3" {
		t.Errorf("php_version = %q after removing the local file, want the base value back", cfg.PHPVersion)
	}
}

func TestLocalOverrideKeys(t *testing.T) {
	dir := t.TempDir()
	writeLocalFixture(t, dir, "php_version: \"8.3\"\n", "domains:\n  - acme-branch\ndb_isolated: true\n")

	keys, err := LocalOverrideKeys(dir)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	want := []string{"db_isolated", "domains"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys = %v, want %v", keys, want)
		}
	}

	if keys, err := LocalOverrideKeys(t.TempDir()); err != nil || keys != nil {
		t.Errorf("no local file: keys = %v, err = %v, want nil, nil", keys, err)
	}
}

// Saving a value the local file overrides is a silent no-op without a word, so
// the user hears which key swallowed their change.
func TestSaveProjectConfig_warnsWhenLocalSwallowsTheChange(t *testing.T) {
	dir := t.TempDir()
	writeLocalFixture(t, dir, "php_version: \"8.3\"\n", "runtime: frankenphp\n")

	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var out strings.Builder
	restore := feedback.SetTestWriter(&out)
	cfg.Runtime = "fpm"
	err = SaveProjectConfig(dir, cfg)
	restore()
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !strings.Contains(out.String(), "runtime") || !strings.Contains(out.String(), LocalOverrideFile) {
		t.Errorf("warning = %q, want the key and the file named", out.String())
	}

	// An ordinary save, where nothing contradicts the local file, stays quiet.
	out.Reset()
	cfg, _ = LoadProjectConfig(dir)
	cfg.Workers = []string{"queue"}
	restore = feedback.SetTestWriter(&out)
	err = SaveProjectConfig(dir, cfg)
	restore()
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if out.String() != "" {
		t.Errorf("unexpected warning on an ordinary save: %q", out.String())
	}
}
