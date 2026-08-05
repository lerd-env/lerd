package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Securing a site has to write the URL where the framework actually keeps it.
// Symfony's base URL is DEFAULT_URI in .env.local, so a hardcoded APP_URL in
// .env misses on both the key and the file and the app keeps generating http
// links for a site served over https.
func TestURLTargetFor(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"symfony/framework-bundle":"^8.0","symfony/runtime":"^8.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	file, key := URLTargetFor(dir)
	if file != ".env.local" || key != "DEFAULT_URI" {
		t.Errorf("URLTargetFor(symfony) = (%q, %q), want (.env.local, DEFAULT_URI)", file, key)
	}

	// An unknown project keeps the Laravel-shaped default rather than nothing.
	plain := t.TempDir()
	file, key = URLTargetFor(plain)
	if file != ".env" || key != "APP_URL" {
		t.Errorf("URLTargetFor(unknown) = (%q, %q), want (.env, APP_URL)", file, key)
	}
}
