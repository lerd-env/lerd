package config

import (
	"encoding/xml"
	"maps"
	"os"
	"path/filepath"
	"slices"

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
	return slices.ContainsFunc(dbTargetsIn(projectDir, bindings, merged), func(t DBTarget) bool {
		return t.Database == db+TestingDBSuffix
	})
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
