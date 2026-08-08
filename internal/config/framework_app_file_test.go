package config

import (
	"os"
	"path/filepath"
	"testing"
)

// A framework whose configuration is not a dotenv file declares the file the
// application itself reads, and lerd reads and writes that one. It is a new
// field rather than a change to the old ones precisely so a binary that predates
// it carries on with what it already understood.
func TestAppFile_isWhatLerdReadsAndWrites(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "web", "sites", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "web/sites/default/settings.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := FrameworkEnvConf{
		File:           ".env",
		Format:         "dotenv",
		FallbackFile:   "web/sites/default/settings.php",
		FallbackFormat: "php-const",
		AppFile:        "web/sites/default/settings.php",
		AppFormat:      "php-vars",
	}

	if file, format := env.ResolveWrite(dir); file != "web/sites/default/settings.php" || format != "php-vars" {
		t.Errorf("ResolveWrite = (%q, %q), want the application's own file", file, format)
	}
	if file, format := env.Resolve(dir); file != "web/sites/default/settings.php" || format != "php-vars" {
		t.Errorf("Resolve = (%q, %q), want the application's own file", file, format)
	}
}

// The same declaration read by a binary that does not know the field resolves
// the way it always did, which is the whole point of adding one.
func TestAppFile_theOlderFieldsStillDescribeTheOldBehaviour(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "web", "sites", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "web/sites/default/settings.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// AppFile omitted, as an older binary's parse of the same yaml leaves it.
	env := FrameworkEnvConf{
		File:           ".env",
		Format:         "dotenv",
		FallbackFile:   "web/sites/default/settings.php",
		FallbackFormat: "php-const",
	}

	if file, format := env.Resolve(dir); file != "web/sites/default/settings.php" || format != "php-const" {
		t.Errorf("Resolve = (%q, %q), want the fallback read as php-const", file, format)
	}
	if file, _ := env.ResolveWrite(dir); file != ".env" {
		t.Errorf("ResolveWrite = %q, want the declared primary", file)
	}
}

// A declared application file that is not there yet is still where lerd writes,
// so a fresh project gets one rather than having its values put elsewhere.
func TestAppFile_writtenEvenBeforeItExists(t *testing.T) {
	env := FrameworkEnvConf{File: ".env", AppFile: "app/etc/config.php", AppFormat: "php-vars"}
	if file, format := env.ResolveWrite(t.TempDir()); file != "app/etc/config.php" || format != "php-vars" {
		t.Errorf("ResolveWrite = (%q, %q), want the application file", file, format)
	}
}
