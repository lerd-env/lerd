package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// statOf identifies the file behind a path, so os.SameFile can tell an
// untouched file from one republished through a temp and a rename.
func statOf(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func ageFile(t *testing.T, path string, age time.Duration) {
	t.Helper()
	stamp := time.Now().Add(-age)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
}

// GetFrameworkForDir refetches a definition once its file is over 24h old, and
// that window is measured from the file's mtime. Skipping the write for
// unchanged bytes must still move the stamp, or every call past the window
// refetches over the network forever instead of once a day.
func TestSaveStoreFramework_refreshesTheStalenessStampWhenContentIsUnchanged(t *testing.T) {
	store := storeSandbox(t)
	path := filepath.Join(store, "acme@1.yaml")

	if err := SaveStoreFramework(storeFrameworkWithWorkers("Acme", 10)); err != nil {
		t.Fatal(err)
	}
	ageFile(t, path, 48*time.Hour)
	before := statOf(t, path)

	if err := SaveStoreFramework(storeFrameworkWithWorkers("Acme", 10)); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(info.ModTime()) > time.Minute {
		t.Errorf("staleness stamp still reads %v old, so the definition refetches on every call", time.Since(info.ModTime()))
	}
	if !os.SameFile(before, statOf(t, path)) {
		t.Error("unchanged definition was republished rather than restamped")
	}
}

// EnsurePreset uses the same 24h window on the preset's own mtime.
func TestSaveStorePreset_refreshesTheStalenessStampWhenContentIsUnchanged(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path := filepath.Join(StorePresetsDir(), "redis.yaml")

	if err := SaveStorePreset("redis", storePresetYAML("1")); err != nil {
		t.Fatal(err)
	}
	ageFile(t, path, 48*time.Hour)
	before := statOf(t, path)

	if err := SaveStorePreset("redis", storePresetYAML("1")); err != nil {
		t.Fatal(err)
	}

	if storePresetStale("redis") {
		t.Error("preset still reads stale after a refresh, so it refetches on every call")
	}
	if !os.SameFile(before, statOf(t, path)) {
		t.Error("unchanged preset was republished rather than restamped")
	}
}
