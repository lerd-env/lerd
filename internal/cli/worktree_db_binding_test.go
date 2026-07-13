package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// resolveDBEnvBinding must derive the env file, format and DB host/name keys from
// the framework definition, so db:isolate addresses each framework's database the
// way that framework actually stores it.
func TestResolveDBEnvBinding(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	storeDir := config.StoreFrameworksDir()
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A Magento-shaped php-array framework: env under app/etc/env.php, database
	// addressed by dotted keys.
	magDef := "name: magish\nlabel: Magish\nenv:\n  file: app/etc/env.php\n  format: php-array\n  url_key: none\n  services:\n    mysql:\n      vars:\n        - db.connection.default.host=lerd-mysql\n        - db.connection.default.dbname={{site}}\n"
	if err := os.WriteFile(filepath.Join(storeDir, "magish.yaml"), []byte(magDef), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name                     string
		framework                string
		wantFile, wantFmt        string
		wantHostKey, wantNameKey string
	}{
		{"laravel", "laravel", ".env", "dotenv", "DB_HOST", "DB_DATABASE"},
		{"magento", "magish", "app/etc/env.php", "php-array", "db.connection.default.host", "db.connection.default.dbname"},
		// Symfony encodes the database in a single DATABASE_URL DSN, so there is no
		// standalone name key; the binding falls back to the Laravel-shaped default.
		{"symfony-dsn-fallback", "symfony", ".env", "dotenv", "DB_HOST", "DB_DATABASE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("framework: "+tc.framework+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
			b := resolveDBEnvBinding(dir)
			if b.file != tc.wantFile || b.format != tc.wantFmt {
				t.Errorf("file/format = %q/%q, want %q/%q", b.file, b.format, tc.wantFile, tc.wantFmt)
			}
			if b.hostKey != tc.wantHostKey || b.nameKey != tc.wantNameKey {
				t.Errorf("hostKey/nameKey = %q/%q, want %q/%q", b.hostKey, b.nameKey, tc.wantHostKey, tc.wantNameKey)
			}
		})
	}
}
