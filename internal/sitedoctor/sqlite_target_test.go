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

// A framework that addresses its database by a dotted path has no Laravel
// default to fall back on. Answering with one sends the user to create a file
// the application will never open, and offers migrations against it.
func TestDeclaredCompanionValue_noLaravelDefaultForADottedKey(t *testing.T) {
	drupalish := &config.Framework{
		Name: "drupalish",
		Env: config.FrameworkEnvConf{File: "settings.php", Services: map[string]config.FrameworkServiceDef{
			"mysql": {
				Detect: []config.FrameworkServiceDetect{{Key: "databases.default.default.driver", ValuePrefix: "mysql"}},
				Vars: []string{
					"databases.default.default.driver=mysql",
					"databases.default.default.database={{site}}",
				},
			},
		}},
	}
	declared := declaredEnvKeys(drupalish)
	vals := map[string]string{
		"databases.default.default.driver":   "sqlite",
		"databases.default.default.database": "",
	}

	if got := declaredCompanionValue(vals, declared, "databases.default.default.driver"); got != "" {
		t.Errorf("companion = %q, want empty: this framework has no database/database.sqlite convention", got)
	}

	// The same empty value under Laravel's own key keeps its default, which is
	// where the convention actually comes from.
	lara := declaredEnvKeys(&config.Framework{Env: config.FrameworkEnvConf{Services: map[string]config.FrameworkServiceDef{
		"sqlite": {Vars: []string{"DB_CONNECTION=sqlite", "DB_DATABASE="}},
	}}})
	got := declaredCompanionValue(map[string]string{"DB_CONNECTION": "sqlite", "DB_DATABASE": ""}, lara, "DB_CONNECTION")
	if got != filepath.Join("database", "database.sqlite") {
		t.Errorf("companion = %q, want Laravel's default", got)
	}
}

// A framework's own detect rules say how a project on a file database reads.
// CakePHP spells the connection as a driver class and CodeIgniter as SQLite3,
// neither of which the generic scan for the word "sqlite" may assume.
func TestSQLiteFileFromValues_ReadsTheDeclaredDetectRules(t *testing.T) {
	for _, tc := range []struct {
		name   string
		detect []config.FrameworkServiceDetect
		vals   map[string]string
		want   string
	}{
		{
			"cakephp driver class",
			[]config.FrameworkServiceDetect{{Key: "Datasources.default.driver", ValuePrefix: `Cake\Database\Driver\Sqlite`}},
			map[string]string{
				"Datasources.default.driver":   `Cake\Database\Driver\Sqlite`,
				"Datasources.default.database": "database/database.sqlite",
			},
			"database/database.sqlite",
		},
		{
			"codeigniter SQLite3",
			[]config.FrameworkServiceDetect{{Key: "database.default.DBDriver", ValuePrefix: "SQLite3"}},
			map[string]string{
				"database.default.DBDriver": "SQLite3",
				"database.default.database": "writable/db.sqlite3",
			},
			"writable/db.sqlite3",
		},
		{
			"laravel flat pair",
			[]config.FrameworkServiceDetect{{Key: "DB_CONNECTION", ValuePrefix: "sqlite"}},
			map[string]string{"DB_CONNECTION": "sqlite", "DB_DATABASE": "database/database.sqlite"},
			"database/database.sqlite",
		},
		{
			"symfony DSN",
			[]config.FrameworkServiceDetect{{Key: "DATABASE_URL", ValuePrefix: "sqlite://"}},
			map[string]string{"DATABASE_URL": "sqlite:///%kernel.project_dir%/var/data.db"},
			"var/data.db",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The store schema ships the wiring vars alongside the detect
			// rules; they are the vocabulary the companion key is found in.
			vars := make([]string, 0, len(tc.vals))
			for k, v := range tc.vals {
				vars = append(vars, k+"="+v)
			}
			fw := &config.Framework{Env: config.FrameworkEnvConf{SQLite: &config.FrameworkServiceDef{
				Detect: tc.detect,
				Vars:   vars,
			}}}
			got, ok := SQLiteFileFromValues(tc.vals, fw)
			if !ok || got != tc.want {
				t.Errorf("resolved %q (ok=%v), want %q", got, ok, tc.want)
			}
		})
	}
}

// A detect rule that does not match must not detect: a MySQL driver value is
// not a file database however the framework spells it.
func TestSQLiteFileFromValues_ARuleThatDoesNotMatchSaysNothing(t *testing.T) {
	fw := &config.Framework{Env: config.FrameworkEnvConf{SQLite: &config.FrameworkServiceDef{
		Detect: []config.FrameworkServiceDetect{{Key: "Datasources.default.driver", ValuePrefix: `Cake\Database\Driver\Sqlite`}},
	}}}
	vals := map[string]string{
		"Datasources.default.driver":   `Cake\Database\Driver\Mysql`,
		"Datasources.default.database": "app",
	}
	if got, ok := SQLiteFileFromValues(vals, fw); ok {
		t.Errorf("a mysql driver detected as sqlite: %q", got)
	}
}

// Where the file gets created follows the same resolution the checks read by:
// never an absolute path or one that already exists, preferably a candidate
// whose parent directory the project already has.
func TestSQLiteCreationTarget(t *testing.T) {
	drupalish := &config.Framework{PublicDir: "web"}

	t.Run("absolute is not lerd's to create", func(t *testing.T) {
		if p, ok := SQLiteCreationTarget(t.TempDir(), nil, "/var/db/app.db"); ok {
			t.Errorf("offered to create %q", p)
		}
	})

	t.Run("an existing file needs nothing", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "database"), 0o755)
		os.WriteFile(filepath.Join(dir, "database", "db.sqlite"), nil, 0o644)
		if p, ok := SQLiteCreationTarget(dir, nil, "database/db.sqlite"); ok {
			t.Errorf("offered to recreate %q", p)
		}
	})

	t.Run("docroot candidate wins when its parent exists", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "web", "sites", "default", "files"), 0o755)
		p, ok := SQLiteCreationTarget(dir, drupalish, filepath.Join("sites", "default", "files", ".ht.sqlite"))
		want := filepath.Join(dir, "web", "sites", "default", "files", ".ht.sqlite")
		if !ok || p != want {
			t.Errorf("target %q (ok=%v), want %q", p, ok, want)
		}
	})

	t.Run("project root stands when no parent exists", func(t *testing.T) {
		dir := t.TempDir()
		p, ok := SQLiteCreationTarget(dir, drupalish, "database/database.sqlite")
		want := filepath.Join(dir, "database", "database.sqlite")
		if !ok || p != want {
			t.Errorf("target %q (ok=%v), want %q", p, ok, want)
		}
	})
}

// A worktree is seeded from the file the parent project really opens, so the
// lookup reports it relative to the project, and only when it exists.
func TestProjectSQLiteFile_findsTheExistingFileRelativeToTheProject(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("DB_CONNECTION=sqlite\nDB_DATABASE=database/app.sqlite\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := ProjectSQLiteFile(project, laravelish()); ok {
		t.Fatal("reported a database that is not on disk")
	}
	if err := os.MkdirAll(filepath.Join(project, "database"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "database", "app.sqlite"), []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}

	rel, ok := ProjectSQLiteFile(project, laravelish())
	if !ok || rel != filepath.Join("database", "app.sqlite") {
		t.Errorf("ProjectSQLiteFile = %q, %v; want database/app.sqlite, true", rel, ok)
	}
}

// An absolute path is one file every checkout already shares, and an in-memory
// database has no file at all, so neither is something to copy.
func TestProjectSQLiteFile_skipsAbsoluteAndInMemory(t *testing.T) {
	for _, db := range []string{filepath.Join(t.TempDir(), "shared.sqlite"), ":memory:"} {
		project := t.TempDir()
		if err := os.WriteFile(filepath.Join(project, ".env"), []byte("DB_CONNECTION=sqlite\nDB_DATABASE="+db+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if filepath.IsAbs(db) {
			if err := os.WriteFile(db, []byte("db"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if rel, ok := ProjectSQLiteFile(project, laravelish()); ok {
			t.Errorf("DB_DATABASE=%s: ProjectSQLiteFile = %q, want none", db, rel)
		}
	}
}
