package sitedoctor

import (
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
	declared := declaredEnvKeys(fw)
	if len(declared) == 0 {
		// A framework that declares no env vocabulary, or no framework at all,
		// leaves nothing to respect: a bare project's .env is read by the
		// convention such a project follows.
		declared = map[string]bool{"DB_CONNECTION": true, "DB_DATABASE": true}
	}
	vals := envfile.Values(envPath, envFormat)

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
		if declared[k] {
			if v := strings.TrimSpace(vals[k]); v != "" {
				return v
			}
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
	for _, def := range fw.Env.Services {
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
