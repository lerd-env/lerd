package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestPHPVersionForDir_PrefersTheRegisteredSite pins the rule that the CLI runs on
// the same PHP as the site's FPM container. lerd link clamps to the framework's
// supported range, so a .php-version outside it would otherwise send composer,
// console and php:shell into a different container than the one serving the site.
func TestPHPVersionForDir_PrefersTheRegisteredSite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// The project asks for 8.1, but the framework clamp registered the site on 8.5.
	if err := os.WriteFile(filepath.Join(dir, ".php-version"), []byte("8.1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: dir, PHPVersion: "8.5"}); err != nil {
		t.Fatal(err)
	}

	got, err := phpVersionForDir(dir)
	if err != nil {
		t.Fatalf("phpVersionForDir: %v", err)
	}
	if got != "8.5" {
		t.Errorf("phpVersionForDir = %q, want %q (the version the site's FPM runs)", got, "8.5")
	}
}

// TestPHPVersionForDir_FallsBackToDetection keeps the unlinked case working: a
// directory that is not a registered site still resolves from the project itself.
func TestPHPVersionForDir_FallsBackToDetection(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := filepath.Join(t.TempDir(), "loose")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".php-version"), []byte("8.1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := phpVersionForDir(dir)
	if err != nil {
		t.Fatalf("phpVersionForDir: %v", err)
	}
	if got != "8.1" {
		t.Errorf("phpVersionForDir = %q, want %q", got, "8.1")
	}
}
