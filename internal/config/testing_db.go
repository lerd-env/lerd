package config

import (
	"encoding/xml"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/envfile"
)

// TestingDBSuffix names the database a project's tests run against, beside its
// own: myapp's is myapp_testing.
const TestingDBSuffix = "_testing"

// DatabasesToCreate is the databases lerd creates for a project using db: db
// itself, and its testing twin unless the project's tests run elsewhere. env is
// the project's env as it is about to be written; nil reads it from disk.
func DatabasesToCreate(projectDir, db string, env map[string]string) []string {
	if !TestingDatabaseWanted(projectDir, db, env) {
		return []string{db}
	}
	return []string{db, db + TestingDBSuffix}
}

// TestingDatabaseWanted reports whether a project's tests use db's testing
// twin, laying the PHPUnit config's env over the project's. A config that says
// nothing about the database still wants it, as it always has.
func TestingDatabaseWanted(projectDir, db string, env map[string]string) bool {
	overrides := phpunitEnv(projectDir)
	bindings, file, format := dbEnvBindings(projectDir)
	if !touchesDBKeys(overrides, bindings) {
		return true
	}
	if env == nil {
		env = envfile.Values(filepath.Join(projectDir, file), format)
	}
	merged := maps.Clone(env)
	if merged == nil {
		merged = map[string]string{}
	}
	maps.Copy(merged, overrides)
	return slices.Contains(testDatabaseNames(bindings, merged), db+TestingDBSuffix)
}

// testDatabaseNames is every database name vals declare through the framework's
// keys. It doesn't ask which host serves them: a native site reaches lerd's
// engine on loopback, so a name is all the decision needs.
func testDatabaseNames(bindings []dbServiceBinding, vals map[string]string) []string {
	read := func(k string) string { return vals[k] }
	var names []string
	for _, b := range bindings {
		if !serviceDetectedIn(b.detect, vals) {
			continue
		}
		if b.URLKey == "" && b.NameKey == "" {
			names = append(names, vals["DB_DATABASE"])
			names = append(names, connectionDatabaseNames(vals)...)
			continue
		}
		names = append(names, b.Database(read))
	}
	if serverDBDriver(vals["DB_CONNECTION"]) {
		names = append(names, vals["DB_DATABASE"])
	}
	for _, k := range []string{"DB_URL", "DATABASE_URL"} {
		if v := vals[k]; v != "" && !strings.HasPrefix(v, "sqlite") {
			names = append(names, DBNameInURL(v))
		}
	}
	return names
}

// connectionDatabaseNames is the database each connection string in vals names,
// whatever key holds it or host it reaches, the fallback for a definition that
// detects a service without saying which keys address it.
func connectionDatabaseNames(vals map[string]string) []string {
	var names []string
	for _, v := range vals {
		if isDBConnectionString(v) {
			names = append(names, DBNameInURL(v))
		}
	}
	return names
}

// isDBConnectionString reports whether v is a URL or PDO DSN for a database
// engine lerd can serve, so an app or asset URL's path is never read as a name.
func isDBConnectionString(v string) bool {
	v = strings.ToLower(strings.Trim(strings.TrimSpace(v), `"'`))
	scheme, rest, ok := strings.Cut(v, ":")
	if !ok || (!strings.HasPrefix(rest, "//") && !strings.Contains(rest, "dbname=")) {
		return false
	}
	scheme = strings.TrimPrefix(strings.TrimPrefix(scheme, "pdo-"), "pdo_")
	switch scheme {
	case "mysql", "mysql2", "mysqli", "mariadb", "pgsql", "postgres", "postgresql", "mongodb", "mongodb+srv":
		return true
	}
	return false
}

// touchesDBKeys reports whether overrides set any key the framework reads its
// database from, or detects its database service by.
func touchesDBKeys(overrides map[string]string, bindings []dbServiceBinding) bool {
	keys := []string{"DB_CONNECTION", "DB_HOST", "DB_DATABASE", "DB_URL", "DATABASE_URL"}
	for _, b := range bindings {
		keys = append(keys, b.keys...)
		for _, d := range b.detect {
			keys = append(keys, d.Key)
		}
	}
	for _, k := range keys {
		if _, ok := overrides[k]; ok {
			return true
		}
	}
	return false
}

// phpunitEnv reads the <env> and <server> values from the PHPUnit config that
// would run, phpunit.xml before the committed .dist files, as PHPUnit picks it.
func phpunitEnv(projectDir string) map[string]string {
	for _, name := range []string{"phpunit.xml", "phpunit.xml.dist", "phpunit.dist.xml"} {
		data, err := os.ReadFile(filepath.Join(projectDir, name))
		if err != nil {
			continue
		}
		var cfg struct {
			PHP struct {
				Vars []struct {
					XMLName xml.Name
					Name    string `xml:"name,attr"`
					Value   string `xml:"value,attr"`
				} `xml:",any"`
			} `xml:"php"`
		}
		if xml.Unmarshal(data, &cfg) != nil {
			return nil
		}
		out := map[string]string{}
		for _, v := range cfg.PHP.Vars {
			if kind := v.XMLName.Local; v.Name != "" && (kind == "env" || kind == "server") {
				out[v.Name] = v.Value
			}
		}
		return out
	}
	return nil
}
