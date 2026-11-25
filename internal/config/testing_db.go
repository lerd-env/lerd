package config

import (
	"encoding/xml"
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

// TestingDatabaseWanted reports whether a project's tests may use db's testing
// twin. Only a PHPUnit config that clearly runs them elsewhere, on SQLite or a
// database by another name, rules it out; anything less keeps it.
func TestingDatabaseWanted(projectDir, db string, env map[string]string) bool {
	tests := phpunitEnv(projectDir)
	if len(tests) == 0 {
		return true
	}
	bindings, file, format := dbEnvBindings(projectDir)
	if env == nil {
		env = envfile.Values(filepath.Join(projectDir, file), format)
	}
	return !testsRunElsewhere(bindings, env, tests, db+TestingDBSuffix)
}

// testsRunElsewhere reports whether the PHPUnit values name a database other
// than twin. An empty value says nothing, and a connection lerd can't read,
// such as one appending the suffix itself, leaves the answer open.
func testsRunElsewhere(bindings []dbServiceBinding, env, tests map[string]string, twin string) bool {
	if isSQLite(tests["DB_CONNECTION"]) {
		return true
	}
	conn := tests["DB_CONNECTION"]
	if conn == "" {
		conn = env["DB_CONNECTION"]
	}
	if !serverDBDriver(conn) {
		return false
	}
	urlKeys, nameKeys := testDBKeys(bindings)
	var names []string
	for _, k := range urlKeys {
		switch v := strings.TrimSpace(tests[k]); {
		case isSQLite(v):
			return true
		case isDBConnectionString(v):
			names = append(names, DBNameInURL(v))
		}
	}
	for _, k := range nameKeys {
		names = append(names, strings.TrimSpace(tests[k]))
	}
	names = slices.DeleteFunc(names, func(n string) bool { return n == "" })
	return len(names) > 0 && !slices.Contains(names, twin)
}

// testDBKeys is the keys a framework reads its database from, as URLs and as
// bare names. A definition that detects a service without saying how it is
// addressed is read through its detect keys.
func testDBKeys(bindings []dbServiceBinding) (urlKeys, nameKeys []string) {
	urlKeys, nameKeys = []string{"DB_URL", "DATABASE_URL"}, []string{"DB_DATABASE"}
	for _, b := range bindings {
		if b.URLKey != "" {
			urlKeys = append(urlKeys, b.URLKey)
		}
		if b.NameKey != "" {
			nameKeys = append(nameKeys, b.NameKey)
		}
		if b.URLKey == "" && b.NameKey == "" {
			for _, d := range b.detect {
				urlKeys = append(urlKeys, d.Key)
			}
		}
	}
	return urlKeys, nameKeys
}

func isSQLite(v string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(v)), "sqlite")
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
