package config

import (
	"os"
	"path/filepath"
	"testing"
)

// db.database is expanded into {{site}}, which preset commands hand to sh -c,
// so a cloned project's .lerd.yaml must not be able to smuggle a command in.
func TestLoadProjectConfigDropsUnsafeDatabaseName(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"example_dk", "example_dk"},
		{"app; id #", ""},
		{"$(id)", ""},
		{"../escape", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			yaml := "db:\n  service: mysql\n  database: '" + tc.name + "'\n"
			if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(yaml), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadProjectConfig(dir)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DB.Database != tc.want {
				t.Errorf("DB.Database = %q, want %q", cfg.DB.Database, tc.want)
			}
		})
	}
}
