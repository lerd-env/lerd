package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A running lerd-ui memoises parsed presets for the process lifetime. Store
// cache files are rewritten by other processes (lerd install, lerd service
// update, the 24h refresh), so a memoised entry must notice the file changed
// on disk and re-parse, or the daemon keeps serving the stale parse until the
// next restart. Issue #1182.
func TestLoadPreset_ReParsesWhenStoreCacheFileMtimeChanges(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// Write a store preset that shadows the embedded redis, parse it, then
	// rewrite the file with different content and a newer mtime. The next
	// LoadPreset must reflect the rewrite, not the cached first parse.
	writeStorePreset(t, "redis", "name: redis\nimage: example/store-redis:v1\ndescription: first\n")
	presetCache.Delete("redis")

	p1, err := LoadPreset("redis")
	if err != nil {
		t.Fatalf("LoadPreset v1: %v", err)
	}
	if p1.Image != "example/store-redis:v1" {
		t.Fatalf("first parse Image = %q, want example/store-redis:v1", p1.Image)
	}

	// Rewrite with new content and bump mtime past the original.
	path := filepath.Join(StorePresetsDir(), "redis.yaml")
	future := time.Now().Add(2 * time.Second)
	writeStorePreset(t, "redis", "name: redis\nimage: example/store-redis:v2\ndescription: second\n")
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	p2, err := LoadPreset("redis")
	if err != nil {
		t.Fatalf("LoadPreset v2: %v", err)
	}
	if p2.Image != "example/store-redis:v2" {
		t.Errorf("stale cache after mtime change: Image = %q, want example/store-redis:v2", p2.Image)
	}
	if p2.Description != "second" {
		t.Errorf("stale cache after mtime change: Description = %q, want second", p2.Description)
	}

	presetCache.Delete("redis")
}

// The embedded bundle has no on-disk file, so its memoised parse must stay
// cached for the process lifetime, never re-parsed on a second LoadPreset.
func TestLoadPreset_EmbeddedBundleStaysCached(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	presetCache.Delete("redis")

	p1, err := LoadPreset("redis")
	if err != nil {
		t.Fatalf("LoadPreset: %v", err)
	}
	p2, err := LoadPreset("redis")
	if err != nil {
		t.Fatalf("second LoadPreset: %v", err)
	}
	if p1 != p2 {
		t.Error("embedded bundle re-parsed on second LoadPreset, expected the same cached pointer")
	}
	presetCache.Delete("redis")
}
