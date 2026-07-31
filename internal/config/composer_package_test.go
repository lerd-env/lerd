package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeComposerJSON writes a composer.json in a fresh dir and returns the dir.
func writeComposerJSON(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(body), 0644); err != nil {
		t.Fatalf("write composer.json: %v", err)
	}
	return dir
}

func resetComposerCache(t *testing.T) {
	t.Helper()
	composerCacheMu.Lock()
	composerCache = map[string]composerCacheEntry{}
	composerCacheMu.Unlock()
}

func TestComposerHasPackage_findsRequireAndRequireDev(t *testing.T) {
	resetComposerCache(t)
	dir := writeComposerJSON(t, `{
	  "require": {"laravel/framework": "^11.0"},
	  "require-dev": {"pestphp/pest": "^2.0"}
	}`)

	if !ComposerHasPackage(dir, "laravel/framework") {
		t.Error("require package not found")
	}
	if !ComposerHasPackage(dir, "pestphp/pest") {
		t.Error("require-dev package not found")
	}
	if ComposerHasPackage(dir, "nope/nope") {
		t.Error("absent package reported present")
	}
}

func TestComposerHasPackage_findsExtraSection(t *testing.T) {
	resetComposerCache(t)
	dir := writeComposerJSON(t, `{
	  "require": {"a/b": "^1.0"},
	  "replace": {"c/d": "*"}
	}`)

	if ComposerHasPackage(dir, "c/d") {
		t.Error("replace must not match without being asked for")
	}
	if !ComposerHasPackage(dir, "c/d", "replace") {
		t.Error("extra section not searched")
	}
}

func TestComposerHasPackage_missingOrMalformedFile(t *testing.T) {
	resetComposerCache(t)
	if ComposerHasPackage(t.TempDir(), "a/b") {
		t.Error("missing composer.json must report false")
	}
	if ComposerHasPackage(writeComposerJSON(t, `not json`), "a/b") {
		t.Error("malformed composer.json must report false")
	}
}

// Sections that aren't package maps (autoload, scripts, nested objects) must be
// skipped rather than aborting the parse.
func TestComposerHasPackage_ignoresNonPackageSections(t *testing.T) {
	resetComposerCache(t)
	dir := writeComposerJSON(t, `{
	  "autoload": {"psr-4": {"App\\": "app/"}},
	  "scripts": {"post-install-cmd": ["@php artisan"]},
	  "require": {"laravel/framework": "^11.0"}
	}`)

	if !ComposerHasPackage(dir, "laravel/framework") {
		t.Error("require not found alongside non-package sections")
	}
}

// The dashboard snapshot asks this per worker rule per site, so repeat lookups
// against an unchanged file must not re-read and re-parse it.
func TestComposerHasPackage_reusesTheParseWhileFileIsUnchanged(t *testing.T) {
	resetComposerCache(t)
	dir := writeComposerJSON(t, `{"require": {"laravel/framework": "^11.0"}}`)

	reads := 0
	orig := composerReadFile
	composerReadFile = func(name string) ([]byte, error) {
		reads++
		return orig(name)
	}
	t.Cleanup(func() { composerReadFile = orig })

	for i := 0; i < 10; i++ {
		if !ComposerHasPackage(dir, "laravel/framework") {
			t.Fatal("package not found")
		}
		if ComposerHasPackage(dir, "other/pkg") {
			t.Fatal("absent package reported present")
		}
	}
	if reads != 1 {
		t.Errorf("composer.json read %d times across 20 lookups, want 1", reads)
	}
}

func TestComposerHasPackage_rereadsAfterTheFileChanges(t *testing.T) {
	resetComposerCache(t)
	dir := writeComposerJSON(t, `{"require": {"laravel/framework": "^11.0"}}`)
	path := filepath.Join(dir, "composer.json")

	if ComposerHasPackage(dir, "laravel/horizon") {
		t.Fatal("horizon present before it was added")
	}

	body := `{"require": {"laravel/framework": "^11.0", "laravel/horizon": "^5.0"}}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	// Move mtime forward so the change is visible even where the filesystem
	// timestamp granularity is coarser than the test's runtime.
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if !ComposerHasPackage(dir, "laravel/horizon") {
		t.Error("edit to composer.json was not picked up")
	}
}

// A same-size edit still has to be seen: mtime alone carries the change.
func TestComposerHasPackage_rereadsOnSameSizeEdit(t *testing.T) {
	resetComposerCache(t)
	dir := writeComposerJSON(t, `{"require": {"aaa/bbb": "^1.0"}}`)
	path := filepath.Join(dir, "composer.json")

	if !ComposerHasPackage(dir, "aaa/bbb") {
		t.Fatal("seed package not found")
	}

	if err := os.WriteFile(path, []byte(`{"require": {"ccc/ddd": "^1.0"}}`), 0644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if ComposerHasPackage(dir, "aaa/bbb") {
		t.Error("stale package still reported after a same-size edit")
	}
	if !ComposerHasPackage(dir, "ccc/ddd") {
		t.Error("new package not seen after a same-size edit")
	}
}
