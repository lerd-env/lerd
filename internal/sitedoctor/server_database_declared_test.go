package sitedoctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Whether the site's database exists is asked through the framework
// declaration, so a project keeping its configuration in a PHP settings file is
// checked like any other. Reading DB_HOST and DB_DATABASE by name meant the
// frameworks least likely to be wired that way were the ones lerd could not
// check at all.
func TestCheckServerDatabase_readsAPHPSettingsFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	store := config.StoreFrameworksDir()
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	def := "name: drupalish\nlabel: Drupalish\npublic_dir: web\nenv:\n" +
		"  app_file: web/sites/default/settings.php\n  app_format: php-vars\n  services:\n    mysql:\n" +
		"      detect:\n        - key: databases.default.default.driver\n          value_prefix: mysql\n" +
		"      vars:\n        - databases.default.default.driver=mysql\n" +
		"        - databases.default.default.host=lerd-mysql\n" +
		"        - databases.default.default.database={{site}}\n"
	if err := os.WriteFile(filepath.Join(store, "drupalish.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "web", "sites", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("framework: drupalish\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "<?php\n$databases['default']['default'] = ['driver' => 'mysql', 'host' => 'lerd-mysql', 'database' => 'shop'];\n"
	if err := os.WriteFile(filepath.Join(dir, "web/sites/default/settings.php"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	fw, ok := config.GetFrameworkForDir("drupalish", dir)
	if !ok {
		t.Fatal("the test definition did not resolve")
	}

	restore := stubDatabaseLister(func(string) ([]string, error) { return []string{"other"}, nil })
	defer restore()
	c, produced := checkServerDatabase(dir, fw)
	if !produced || c.Status != StatusFail {
		t.Fatalf("check = %+v (produced=%v), want a failure for the missing schema", c, produced)
	}

	restore()
	restore2 := stubDatabaseLister(func(string) ([]string, error) { return []string{"shop"}, nil })
	defer restore2()
	if c, _ := checkServerDatabase(dir, fw); c.Status != StatusOK {
		t.Errorf("check = %+v, want ok once the database exists", c)
	}
}

// Asking one engine for its databases costs a container exec, and the doctor's
// site sweep asks the same engine once per site, so the answer is reused.
func TestCachedDatabases_ReusesOneLookupPerEngine(t *testing.T) {
	calls := 0
	restore := stubDatabaseLister(func(string) ([]string, error) {
		calls++
		return []string{"shop"}, nil
	})
	defer restore()

	for range 3 {
		if names, err := cachedDatabases("mysql"); err != nil || len(names) != 1 {
			t.Fatalf("got %v, %v", names, err)
		}
	}
	if calls != 1 {
		t.Errorf("looked the engine up %d times, want 1", calls)
	}
	if _, err := cachedDatabases("postgres"); err != nil || calls != 2 {
		t.Errorf("a second engine must be its own lookup: calls=%d err=%v", calls, err)
	}
}
