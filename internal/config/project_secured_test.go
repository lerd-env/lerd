package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Securing a project that has no .lerd.yaml yet has to record it: the registry
// entry goes with lerd unlink, and a later link reads HTTPS from this file.
func TestSetProjectSecured_CreatesTheFileToRecordHTTPS(t *testing.T) {
	dir := t.TempDir()

	if err := SetProjectSecured(dir, true); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Secured {
		t.Error("secured was not recorded for a project with no .lerd.yaml")
	}
}

// Turning HTTPS off is the default already, so it is no reason to create one.
func TestSetProjectSecured_UnsecureCreatesNothing(t *testing.T) {
	dir := t.TempDir()

	if err := SetProjectSecured(dir, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".lerd.yaml")); err == nil {
		t.Error("unsecuring created a .lerd.yaml")
	}
}
