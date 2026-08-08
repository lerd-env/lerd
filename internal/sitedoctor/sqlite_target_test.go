package sitedoctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func laravelish() *config.Framework {
	return &config.Framework{
		Name: "laravelish",
		Env: config.FrameworkEnvConf{File: ".env", Services: map[string]config.FrameworkServiceDef{
			"mysql": {
				Detect: []config.FrameworkServiceDetect{{Key: "DB_CONNECTION", ValuePrefix: "mysql"}},
				Vars:   []string{"DB_CONNECTION=mysql", "DB_HOST=lerd-mysql", "DB_DATABASE={{site}}"},
			},
		}},
	}
}

func symfonyish() *config.Framework {
	return &config.Framework{
		Name: "symfonyish",
		Env: config.FrameworkEnvConf{File: ".env.local", Services: map[string]config.FrameworkServiceDef{
			"mysql": {
				Detect: []config.FrameworkServiceDetect{{Key: "DATABASE_URL", ValuePrefix: "mysql://"}},
				Vars:   []string{"DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/{{site}}"},
			},
		}},
	}
}

func envAt(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A Symfony project carrying Laravel's keys as leftovers was reported against a
// file it never opens, and offered migrations for a database that isn't its
// own. Its framework declares neither key, so neither is read.
func TestDeclaredSQLiteFile_ignoresKeysTheFrameworkNeverDeclares(t *testing.T) {
	env := envAt(t, "DATABASE_URL=postgresql://postgres:lerd@lerd-postgres:5432/app\nDB_CONNECTION=sqlite\nDB_DATABASE=database/database.sqlite\n")

	if file, ok := declaredSQLiteFile(env, "dotenv", symfonyish()); ok {
		t.Errorf("declaredSQLiteFile = %q, want none: neither key belongs to this framework", file)
	}
}

// The same project genuinely on SQLite is found through the key it does
// declare, with Symfony's project-root placeholder resolved away.
func TestDeclaredSQLiteFile_readsADeclaredDSN(t *testing.T) {
	env := envAt(t, `DATABASE_URL="sqlite:///%kernel.project_dir%/var/app.db"`+"\n")

	file, ok := declaredSQLiteFile(env, "dotenv", symfonyish())
	if !ok || file != "var/app.db" {
		t.Errorf("declaredSQLiteFile = %q (ok=%v), want var/app.db", file, ok)
	}
}

// A framework that does declare the flat pair is read through it as before.
func TestDeclaredSQLiteFile_readsTheDeclaredFlatPair(t *testing.T) {
	env := envAt(t, "DB_CONNECTION=sqlite\nDB_DATABASE=storage/app.sqlite\n")

	file, ok := declaredSQLiteFile(env, "dotenv", laravelish())
	if !ok || file != "storage/app.sqlite" {
		t.Errorf("declaredSQLiteFile = %q (ok=%v), want storage/app.sqlite", file, ok)
	}
}

// A project on a server database has no SQLite file, so there is nothing to
// check and no finding to raise.
func TestDeclaredSQLiteFile_noneForAServerDatabase(t *testing.T) {
	env := envAt(t, "DB_CONNECTION=mysql\nDB_DATABASE=app\n")

	if file, ok := declaredSQLiteFile(env, "dotenv", laravelish()); ok {
		t.Errorf("declaredSQLiteFile = %q, want none for a mysql project", file)
	}
}

// A framework configured through a nested file spells the same pair as paths,
// and the companion is found in the same block rather than by Laravel's name.
func TestDeclaredSQLiteFile_readsDottedKeys(t *testing.T) {
	drupalish := &config.Framework{
		Name: "drupalish",
		Env: config.FrameworkEnvConf{File: "settings.php", Format: "php-vars", Services: map[string]config.FrameworkServiceDef{
			"mysql": {
				Detect: []config.FrameworkServiceDetect{{Key: "databases.default.default.driver", ValuePrefix: "mysql"}},
				Vars: []string{
					"databases.default.default.driver=mysql",
					"databases.default.default.database={{site}}",
				},
			},
		}},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.php")
	body := "<?php\n$databases['default']['default'] = ['driver' => 'sqlite', 'database' => 'sites/default/files/.ht.sqlite'];\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	file, ok := declaredSQLiteFile(path, "php-vars", drupalish)
	if !ok || file != "sites/default/files/.ht.sqlite" {
		t.Errorf("declaredSQLiteFile = %q (ok=%v), want the path in the same block", file, ok)
	}
}
