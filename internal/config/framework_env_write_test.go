package config

import (
	"os"
	"path/filepath"
	"testing"
)

// A definition that names a primary env file means that file is lerd's to
// write. Drupal's settings.php is declared as a fallback so an installed site
// can be read, and writing there appends constants Drupal never looks at while
// the .env its own install command sources is never created.
func TestResolveWrite_declaredPrimaryWinsOverAnExistingFallback(t *testing.T) {
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
	}

	if file, format := env.ResolveWrite(dir); file != ".env" || format != "dotenv" {
		t.Errorf("ResolveWrite = (%q, %q), want (.env, dotenv)", file, format)
	}
	// Reading still finds the installed site's own configuration.
	if file, format := env.Resolve(dir); file != "web/sites/default/settings.php" || format != "php-const" {
		t.Errorf("Resolve = (%q, %q), want the fallback", file, format)
	}
}

// A definition that names no primary has only its fallback, and that fallback
// is the configuration itself: WordPress keeps everything in wp-config.php and
// lerd must go on writing it rather than inventing a .env beside it.
func TestResolveWrite_fallbackIsTheConfigWhenNoPrimaryIsDeclared(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wp-config.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := FrameworkEnvConf{FallbackFile: "wp-config.php", FallbackFormat: "php-const"}

	if file, format := env.ResolveWrite(dir); file != "wp-config.php" || format != "php-const" {
		t.Errorf("ResolveWrite = (%q, %q), want (wp-config.php, php-const)", file, format)
	}
}

// The common shape: one declared file, present or not, written either way.
func TestResolveWrite_declaredPrimaryIsReturnedWhenMissing(t *testing.T) {
	env := FrameworkEnvConf{File: ".env.local", Format: "dotenv"}
	if file, _ := env.ResolveWrite(t.TempDir()); file != ".env.local" {
		t.Errorf("ResolveWrite = %q, want .env.local even though it does not exist yet", file)
	}
}

func TestResolveWrite_defaultsToDotenv(t *testing.T) {
	env := FrameworkEnvConf{}
	if file, format := env.ResolveWrite(t.TempDir()); file != ".env" || format != "dotenv" {
		t.Errorf("ResolveWrite = (%q, %q), want (.env, dotenv)", file, format)
	}
}
