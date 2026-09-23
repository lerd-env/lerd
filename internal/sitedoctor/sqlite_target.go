package sitedoctor

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
)

// declaredSQLiteFile returns the SQLite file a project is configured to open,
// read only through keys its own framework declares.
//
// The check used to read DB_CONNECTION and DB_DATABASE wherever it found them,
// which are Laravel's key names. A Symfony project keeps its SQLite path inside
// the DATABASE_URL DSN and has neither key, so the check either said nothing or,
// where those keys survived as leftovers, reported a file the application never
// opens and offered to migrate it. Asking only about declared keys is what keeps
// one framework's vocabulary from being read into another's project.
//
// ok is false when the project is not configured for SQLite at all, which is
// most of them.
func declaredSQLiteFile(envPath, envFormat string, fw *config.Framework) (string, bool) {
	return SQLiteFileFromValues(envfile.Values(envPath, envFormat), fw)
}

// SQLiteFileFromValues is declaredSQLiteFile against values already in hand,
// which is what `lerd env` has: it knows what it is about to write before the
// file says it, and the file it creates has to be the one those values name.
func SQLiteFileFromValues(vals map[string]string, fw *config.Framework) (string, bool) {
	declared := declaredEnvKeys(fw)
	if len(declared) == 0 {
		// A framework that declares no env vocabulary, or no framework at all,
		// leaves nothing to respect: a bare project's .env is read by the
		// convention such a project follows.
		declared = map[string]bool{"DB_CONNECTION": true, "DB_DATABASE": true}
	}

	// A framework with a sqlite wiring of its own says exactly how a project on
	// a file database reads: its detect rules. CakePHP spells the connection
	// Cake\Database\Driver\Sqlite and CodeIgniter SQLite3, neither of which a
	// generic scan for the word "sqlite" is entitled to assume.
	if fw != nil && fw.Env.SQLite != nil {
		for _, rule := range fw.Env.SQLite.Detect {
			if rule.Key == "" {
				continue
			}
			val, exists := vals[rule.Key]
			if !exists || (rule.ValuePrefix != "" && !strings.HasPrefix(val, rule.ValuePrefix)) {
				continue
			}
			// A DSN-shaped rule carries the path in the matched value itself;
			// a driver-shaped one names it in the database key beside it.
			if file := sqliteDSNPath(val); file != "" {
				return file, true
			}
			if file := declaredCompanionValue(vals, declared, rule.Key); file != "" {
				return file, true
			}
		}
	}

	// The flat shape: a declared key names the connection, another names the
	// file. Laravel spells them DB_CONNECTION and DB_DATABASE; a framework
	// storing its config as a nested array spells the same pair as paths.
	for key, val := range vals {
		if !declared[key] || !strings.EqualFold(strings.TrimSpace(val), "sqlite") {
			continue
		}
		if file := declaredCompanionValue(vals, declared, key); file != "" {
			return file, true
		}
	}

	// The DSN shape: one declared key carries scheme and path together.
	for key, val := range vals {
		if !declared[key] {
			continue
		}
		if file := sqliteDSNPath(val); file != "" {
			return file, true
		}
	}
	return "", false
}

// declaredCompanionValue finds the declared key naming the database file that
// sits beside the one naming the connection: DB_CONNECTION is answered by
// DB_DATABASE, and a dotted driver key by the database key in the same block.
func declaredCompanionValue(vals map[string]string, declared map[string]bool, connKey string) string {
	candidates := []string{"DB_DATABASE"}
	// Laravel's own default, which its .env leaves unset more often than not.
	defaultFile := filepath.Join("database", "database.sqlite")
	if i := strings.LastIndex(connKey, "."); i >= 0 {
		candidates = append([]string{connKey[:i+1] + "database"}, candidates...)
	}
	for _, k := range candidates {
		if !declared[k] {
			continue
		}
		if v := strings.TrimSpace(vals[k]); v != "" {
			return v
		}
		// An empty value only has a default where the convention exists. That is
		// Laravel's, spelled DB_DATABASE; a framework that addresses its database
		// by a dotted path has no such default, and inventing one sends the user
		// to create a file the application will never open.
		if k == "DB_DATABASE" {
			return defaultFile
		}
	}
	return ""
}

// sqliteDSNPath returns the file a SQLite connection string points at, or empty
// for anything else. Symfony writes sqlite:///%kernel.project_dir%/var/app.db,
// and that placeholder is the project root the caller already knows.
func sqliteDSNPath(value string) string {
	v := strings.Trim(strings.TrimSpace(value), `"'`)
	rest, ok := strings.CutPrefix(v, "sqlite://")
	if !ok {
		if rest, ok = strings.CutPrefix(v, "sqlite:"); !ok {
			return ""
		}
	}
	rest = strings.TrimPrefix(rest, "/")
	rest = strings.ReplaceAll(rest, "%kernel.project_dir%/", "")
	rest = strings.ReplaceAll(rest, "%kernel.project_dir%", "")
	if rest == "" || rest == ":memory:" {
		return ""
	}
	return rest
}

// declaredEnvKeys is every env key a framework names, across its service
// detection rules, its service vars and its unconditional vars. It is the
// project's whole vocabulary, so a key outside it means nothing here however
// familiar it looks.
func declaredEnvKeys(fw *config.Framework) map[string]bool {
	if fw == nil {
		return nil
	}
	keys := map[string]bool{}
	add := func(kv string) {
		if k, _, found := strings.Cut(kv, "="); found {
			keys[strings.TrimSpace(k)] = true
		}
	}
	for _, kv := range fw.Env.Vars {
		add(kv)
	}
	defs := make([]config.FrameworkServiceDef, 0, len(fw.Env.Services)+1)
	for _, def := range fw.Env.Services {
		defs = append(defs, def)
	}
	// The file database is wired through keys of its own, and they are as much
	// the project's vocabulary as any service's.
	if fw.Env.SQLite != nil {
		defs = append(defs, *fw.Env.SQLite)
	}
	for _, def := range defs {
		for _, rule := range def.Detect {
			if rule.Key != "" {
				keys[rule.Key] = true
			}
		}
		for _, kv := range def.Vars {
			add(kv)
		}
	}
	if fw.Env.KeyGeneration != nil && fw.Env.KeyGeneration.EnvKey != "" {
		keys[fw.Env.KeyGeneration.EnvKey] = true
	}
	return keys
}

// sqliteFilePaths returns where a declared SQLite file could be, because a
// relative path is relative to whatever the application calls its root and the
// frameworks disagree: Laravel resolves database/database.sqlite against the
// project, Drupal resolves sites/default/files/.ht.sqlite against its document
// root, which is a directory down. Both are checked, and a database found at
// either is the site's, so a healthy 20 MB file is not reported missing because
// lerd measured from the wrong end.
// SQLiteCreationTarget returns where `lerd env` should create a declared
// SQLite file, by the same rules the checks above read by. Nothing to create
// when the file already exists anywhere the application could open it, or when
// the declared path is absolute: that file is the user's own to manage, not
// lerd's to build a directory tree for. Among the candidates, one whose parent
// directory already exists is where the project keeps such files (Laravel's
// database/, Drupal's files dir under the docroot); failing that, the project
// root resolution stands.
func SQLiteCreationTarget(projectPath string, fw *config.Framework, dbFile string) (string, bool) {
	if filepath.IsAbs(dbFile) {
		return "", false
	}
	paths := sqliteFilePaths(projectPath, publicDirOf(fw), dbFile)
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return "", false
		}
	}
	for _, p := range paths {
		if fi, err := os.Stat(filepath.Dir(p)); err == nil && fi.IsDir() {
			return p, true
		}
	}
	return paths[0], true
}

// ProjectSQLiteFile returns the SQLite file the project at path opens, relative
// to path, when its env names one by a relative path and it exists. An absolute
// path is already shared by every checkout, so it is not reported.
func ProjectSQLiteFile(path string, fw *config.Framework) (string, bool) {
	envFile, envFormat, _ := envSetup(fw, path)
	dbFile, ok := declaredSQLiteFile(filepath.Join(path, envFile), envFormat, fw)
	if !ok || dbFile == ":memory:" || filepath.IsAbs(dbFile) {
		return "", false
	}
	for _, abs := range sqliteFilePaths(path, publicDirOf(fw), filepath.FromSlash(dbFile)) {
		if info, err := os.Stat(abs); err == nil && info.Mode().IsRegular() {
			rel, err := filepath.Rel(path, abs)
			return rel, err == nil
		}
	}
	return "", false
}

func sqliteFilePaths(projectPath, publicDir, dbFile string) []string {
	if filepath.IsAbs(dbFile) {
		return []string{dbFile}
	}
	paths := []string{filepath.Join(projectPath, dbFile)}
	if publicDir != "" && publicDir != "." {
		paths = append(paths, filepath.Join(projectPath, publicDir, dbFile))
	}
	return paths
}
