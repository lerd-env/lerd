package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The backup sits next to the file it was taken from. For a plain dotenv
// project that is the long-standing .env.before_lerd at the project root; for a
// framework whose configuration lives elsewhere it follows the file, so a
// CakePHP app_local.php is never stored under a dotenv name at the root.
func TestEnvBackupPath(t *testing.T) {
	cases := []struct{ env, want string }{
		{".env", ".env.before_lerd"},
		{".env.local", ".env.local.before_lerd"},
		{"config/app_local.php", "config/app_local.php.before_lerd"},
		{"wp-config.php", "wp-config.php.before_lerd"},
		{"app/etc/env.php", "app/etc/env.php.before_lerd"},
	}
	for _, c := range cases {
		if got := envBackupPath(c.env); got != c.want {
			t.Errorf("envBackupPath(%q) = %q, want %q", c.env, got, c.want)
		}
	}
}

// Restoring puts the backup back over the file it came from. Writing it to a
// root .env instead left the real configuration untouched and dropped PHP
// source into a file the dotenv reader would then try to parse.
func TestRestoreEnvBackup_WritesTheFrameworkFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0755); err != nil {
		t.Fatal(err)
	}
	original := "<?php\nreturn ['Datasources' => ['default' => ['host' => 'db.example']]];\n"
	writeFile(t, dir, "config/app_local.php.before_lerd", original)
	writeFile(t, dir, "config/app_local.php", "<?php\nreturn ['lerd wrote this'];\n")

	if err := restoreEnvBackup(dir, "config/app_local.php"); err != nil {
		t.Fatalf("restoreEnvBackup: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "config", "app_local.php"))
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(got) != original {
		t.Errorf("restored file = %q, want %q", got, original)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(err) {
		t.Error("restore invented a root .env for a framework that has none")
	}
}

func TestRestoreEnvBackup_MissingBackup(t *testing.T) {
	dir := t.TempDir()
	err := restoreEnvBackup(dir, "config/app_local.php")
	if err == nil {
		t.Fatal("expected an error when no backup exists")
	}
	if !strings.Contains(err.Error(), "config/app_local.php.before_lerd") {
		t.Errorf("error should name the backup it looked for, got %q", err)
	}
}

// The plain dotenv case keeps working exactly as before, so a backup an older
// lerd wrote still restores.
func TestRestoreEnvBackup_PlainDotenvUnchanged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env.before_lerd", "DB_HOST=127.0.0.1\n")
	writeFile(t, dir, ".env", "DB_HOST=lerd-mysql\n")

	if err := restoreEnvBackup(dir, ".env"); err != nil {
		t.Fatalf("restoreEnvBackup: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if string(got) != "DB_HOST=127.0.0.1\n" {
		t.Errorf("restored .env = %q", got)
	}
}
