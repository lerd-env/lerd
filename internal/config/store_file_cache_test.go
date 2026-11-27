package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func countStoreReads(t *testing.T) *int {
	t.Helper()
	resetStoreFileCache(t)
	reads := 0
	orig := storeReadFile
	storeReadFile = func(name string) ([]byte, error) {
		reads++
		return orig(name)
	}
	t.Cleanup(func() { storeReadFile = orig })
	return &reads
}

func touchForward(t *testing.T, path string) {
	t.Helper()
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
}

// Framework detection loads package definitions for every site on every
// watcher tick, so an unchanged file must be parsed once, not per call.
func TestLoadPackageYAML_reusesTheParseUntilTheFileChanges(t *testing.T) {
	reads := countStoreReads(t)
	path := filepath.Join(t.TempDir(), "horizon.yaml")
	if err := os.WriteFile(path, []byte("package: laravel/horizon\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if pkg := loadPackageYAML(path); pkg == nil || pkg.Package != "laravel/horizon" {
			t.Fatalf("load %d: got %+v", i, pkg)
		}
	}
	if *reads != 1 {
		t.Errorf("read %d times across 10 loads, want 1", *reads)
	}

	if err := os.WriteFile(path, []byte("package: laravel/pulse\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	touchForward(t, path)
	if pkg := loadPackageYAML(path); pkg == nil || pkg.Package != "laravel/pulse" {
		t.Fatalf("after a rewrite got %+v, want laravel/pulse", pkg)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if pkg := loadPackageYAML(path); pkg != nil {
		t.Fatalf("after removal got %+v, want nil", pkg)
	}
}

func TestLoadCachedStoreIndex_reusesTheParseUntilTheFileChanges(t *testing.T) {
	reads := countStoreReads(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path := StoreIndexFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"packages":[{"name":"laravel/horizon"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if got := cachedStorePackages(); len(got) != 1 || got[0].Name != "laravel/horizon" {
			t.Fatalf("load %d: got %+v", i, got)
		}
	}
	if *reads != 1 {
		t.Errorf("read %d times across 10 loads, want 1", *reads)
	}
	if err := os.WriteFile(path, []byte(`{"packages":[{"name":"laravel/pulse"},{"name":"laravel/horizon"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	touchForward(t, path)
	if got := cachedStorePackages(); len(got) != 2 {
		t.Fatalf("after a rewrite got %+v, want two packages", got)
	}
}

func resetStoreFileCache(t *testing.T) {
	t.Helper()
	storeFileMu.Lock()
	storeFileCache = map[string]storeFileEntry{}
	storeFileMu.Unlock()
}
