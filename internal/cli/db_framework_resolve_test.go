package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// resolveDBFromFramework must read the framework's env file through the
// version-aware store definition (GetFrameworkForDir), not the Go built-in
// (GetFramework). For Symfony the store def targets .env.local while the
// built-in targets .env, so a DATABASE_URL that lives only in .env.local is
// invisible unless the store def is consulted.
func TestResolveDBFromFramework_ReadsStoreEnvFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	storeDir := config.StoreFrameworksDir()
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	storeDef := "name: symfony\nlabel: Symfony\nenv:\n  file: .env.local\n  format: dotenv\n  services:\n    mysql:\n      detect:\n        - key: DATABASE_URL\n          value_prefix: mysql\n"
	if err := os.WriteFile(filepath.Join(storeDir, "symfony.yaml"), []byte(storeDef), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("framework: symfony\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The committed .env carries no DATABASE_URL; the real value lives in the
	// gitignored .env.local, exactly as Symfony's convention prescribes.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_ENV=dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), []byte("DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/shop\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := resolveDBFromFramework(dir)
	if env == nil {
		t.Fatal("resolveDBFromFramework read the built-in .env and found no service; it must resolve the store def's .env.local")
	}
	if env.connection != "mysql" {
		t.Errorf("connection = %q, want mysql", env.connection)
	}
	if env.database != "shop" {
		t.Errorf("database = %q, want shop (parsed from DATABASE_URL in .env.local)", env.database)
	}
}
