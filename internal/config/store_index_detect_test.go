package config

import (
	"os"
	"path/filepath"
	"testing"
)

// A framework with no built-in Go adapter and no store YAML on disk must still
// be detected, by name and version, from the locally cached store index alone.
// This is the offline case that the built-in fallback can't cover (Drupal,
// Statamic, CakePHP, Tempest, etc.).
func TestDetection_FromCachedStoreIndex(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "composer.json"), //nolint:errcheck
		[]byte(`{"require":{"drupal/core-recommended":"^11.0"}}`), 0644)

	os.MkdirAll(StoreFrameworksDir(), 0755) //nolint:errcheck
	index := `{"frameworks":[{"name":"drupal","label":"Drupal","versions":["11","10"],"latest":"11","detect":[{"composer":"drupal/core-recommended"},{"composer":"drupal/core"}]}]}`
	os.WriteFile(StoreIndexFile(), []byte(index), 0644) //nolint:errcheck

	if name, ok := DetectFramework(dir); !ok || name != "drupal" {
		t.Errorf("DetectFramework = %q, %v; want drupal, true", name, ok)
	}
	if v := DetectMajorVersion(dir, "drupal"); v != "11" {
		t.Errorf("DetectMajorVersion = %q; want 11", v)
	}
}
