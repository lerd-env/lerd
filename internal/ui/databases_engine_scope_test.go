package ui

import (
	"os"
	"path/filepath"
	"testing"
)

// writeEngineService installs a custom service definition into a temp config dir
// so the engine-scope predicate reads a real declaration.
func writeEngineService(t *testing.T, name, body string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "lerd", "services")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestIsDatabaseEngineAcceptsStoreDeclaredEngine(t *testing.T) {
	writeEngineService(t, "analytics", `name: analytics
image: example/analytics:1
introspect:
  entities:
    - kind: databases
      list: list-dbs
      actions:
        create: make-db {{name}}
`)
	if !isDatabaseEngine("analytics") {
		t.Error("an engine outside the wired families must qualify on its own declaration")
	}
}

func TestIsDatabaseEngineRejectsNonEngines(t *testing.T) {
	writeEngineService(t, "objectstore", `name: objectstore
image: example/store:1
introspect:
  entities:
    - kind: buckets
      list: list-buckets
`)
	if isDatabaseEngine("objectstore") {
		t.Error("a service holding buckets is not a database engine")
	}
	if isDatabaseEngine("mailpit") {
		t.Error("a service declaring no entities is not a database engine")
	}
}

func TestIsDatabaseEngineKeepsWiredFamilies(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, name := range []string{"mysql", "postgres"} {
		if !isDatabaseEngine(name) {
			t.Errorf("%s must stay a database engine", name)
		}
	}
}
