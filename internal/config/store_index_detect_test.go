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

// A framework built on another one matches that one's rules too (a Sulu project
// ships symfony.lock). Declaring based_on makes it win over its base whichever
// of the two is on disk and whichever only sits in the cached index.
func TestDetection_BasedOnBeatsItsBase(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "symfony.lock"), []byte("{}"), 0644) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "composer.json"),                    //nolint:errcheck
		[]byte(`{"require":{"sulu/sulu":"^3.0"}}`), 0644)

	os.MkdirAll(StoreFrameworksDir(), 0755)                             //nolint:errcheck
	os.WriteFile(filepath.Join(StoreFrameworksDir(), "symfony@7.yaml"), //nolint:errcheck
		[]byte("name: symfony\nversion: \"7\"\ndetect:\n  - file: symfony.lock\n"), 0644)

	t.Run("derived only in the cached index", func(t *testing.T) {
		index := `{"frameworks":[{"name":"sulu","label":"Sulu","versions":["3"],"latest":"3","based_on":"symfony","detect":[{"composer":"sulu/sulu"}]}]}`
		os.WriteFile(StoreIndexFile(), []byte(index), 0644) //nolint:errcheck
		if name, ok := DetectFramework(dir); !ok || name != "sulu" {
			t.Errorf("DetectFramework = %q, %v; want sulu, true", name, ok)
		}
	})

	// Named to sort after its base, so file order cannot be what picks it.
	t.Run("derived on disk after its base", func(t *testing.T) {
		os.Remove(StoreIndexFile())                                      //nolint:errcheck
		os.WriteFile(filepath.Join(StoreFrameworksDir(), "zeta@1.yaml"), //nolint:errcheck
			[]byte("name: zeta\nversion: \"1\"\nbased_on: symfony\ndetect:\n  - composer: sulu/sulu\n"), 0644)
		if name, ok := DetectFramework(dir); !ok || name != "zeta" {
			t.Errorf("DetectFramework = %q, %v; want zeta, true", name, ok)
		}
	})
}

// based_on must not steal a project of the base: a plain Symfony app stays Symfony.
func TestDetection_BasedOnLeavesItsBaseAlone(t *testing.T) {
	setConfigDir(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "symfony.lock"), []byte("{}"), 0644) //nolint:errcheck

	os.MkdirAll(StoreFrameworksDir(), 0755)                             //nolint:errcheck
	os.WriteFile(filepath.Join(StoreFrameworksDir(), "symfony@7.yaml"), //nolint:errcheck
		[]byte("name: symfony\nversion: \"7\"\ndetect:\n  - file: symfony.lock\n"), 0644)
	index := `{"frameworks":[{"name":"sulu","label":"Sulu","versions":["3"],"latest":"3","based_on":"symfony","detect":[{"composer":"sulu/sulu"}]}]}`
	os.WriteFile(StoreIndexFile(), []byte(index), 0644) //nolint:errcheck

	if name, ok := DetectFramework(dir); !ok || name != "symfony" {
		t.Errorf("DetectFramework = %q, %v; want symfony, true", name, ok)
	}
}
