package sitedoctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A relative database path is relative to whatever the application calls its
// root, and the frameworks disagree. Drupal resolves sites/default/files/.ht.sqlite
// against its document root, so measuring from the project reported a healthy
// database as missing on a site that was serving fine.
func TestCheckSQLiteDatabase_findsADatabaseUnderTheDocumentRoot(t *testing.T) {
	dir := t.TempDir()
	fw := &config.Framework{
		Name:      "drupalish",
		PublicDir: "web",
		Env: config.FrameworkEnvConf{File: "web/sites/default/settings.php", Format: "php-vars", Services: map[string]config.FrameworkServiceDef{
			"mysql": {Vars: []string{"databases.default.default.driver=mysql", "databases.default.default.database={{site}}"}},
		}},
	}
	settings := filepath.Join(dir, "web", "sites", "default", "settings.php")
	if err := os.MkdirAll(filepath.Join(dir, "web", "sites", "default", "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "<?php\n$databases['default']['default'] = ['driver' => 'sqlite', 'database' => 'sites/default/files/.ht.sqlite'];\n"
	if err := os.WriteFile(settings, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// The database as Drupal writes it: under the document root, with content.
	if err := os.WriteFile(filepath.Join(dir, "web", "sites", "default", "files", ".ht.sqlite"), []byte("SQLite format 3\x00"), 0o644); err != nil {
		t.Fatal(err)
	}

	c, ok := checkSQLiteDatabase(dir, settings, "php-vars", fw)
	if !ok || c.Status != StatusOK {
		t.Errorf("check = %+v (ok=%v), want ok: the database is there, under the document root", c, ok)
	}
}

// A project rooting its database at the project directory is unaffected.
func TestCheckSQLiteDatabase_stillFindsOneAtTheProjectRoot(t *testing.T) {
	dir := t.TempDir()
	fw := &config.Framework{Name: "laravelish", PublicDir: "public"}
	if err := os.MkdirAll(filepath.Join(dir, "database"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "database", "database.sqlite"), []byte("SQLite format 3\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("DB_CONNECTION=sqlite\nDB_DATABASE=database/database.sqlite\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c, ok := checkSQLiteDatabase(dir, env, "dotenv", fw)
	if !ok || c.Status != StatusOK {
		t.Errorf("check = %+v (ok=%v), want ok", c, ok)
	}
}

// Absent from both roots is still a real finding.
func TestCheckSQLiteDatabase_missingFromEveryRoot(t *testing.T) {
	dir := t.TempDir()
	fw := &config.Framework{Name: "laravelish", PublicDir: "public"}
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("DB_CONNECTION=sqlite\nDB_DATABASE=database/database.sqlite\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c, ok := checkSQLiteDatabase(dir, env, "dotenv", fw)
	if !ok || c.Status != StatusFail {
		t.Errorf("check = %+v (ok=%v), want a failure", c, ok)
	}
}
