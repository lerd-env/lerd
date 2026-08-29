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

// A declared application file that its installer has not written yet is not
// lerd's to create: the framework reads the skeleton left in its place as a site
// already configured and fails instead of serving the installer, so the values
// go to the plain file the definition names until the installer has run.
func TestAppFile_waitsForTheInstallerWhenAPlainFileIsDeclared(t *testing.T) {
	env := FrameworkEnvConf{File: ".env", AppFile: "app/etc/config.php", AppFormat: "php-vars"}
	if file, format := env.ResolveWrite(t.TempDir()); file != ".env" || format != "dotenv" {
		t.Errorf("ResolveWrite = (%q, %q), want the plain env file", file, format)
	}
}

// With no plain file to fall back on the application file is the configuration,
// which is WordPress's wp-config.php, and lerd creates it as it always has.
func TestAppFile_writtenBeforeItExistsWhenItIsTheOnlyFile(t *testing.T) {
	env := FrameworkEnvConf{AppFile: "wp-config.php", AppFormat: "php-const"}
	if file, format := env.ResolveWrite(t.TempDir()); file != "wp-config.php" || format != "php-const" {
		t.Errorf("ResolveWrite = (%q, %q), want the application file", file, format)
	}
}
