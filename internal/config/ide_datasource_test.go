package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGlobal(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "lerd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lerd", "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	invalidateGlobalCache()
}

// Writing into a project only ever touches a directory the IDE already made,
// and only lerd's own entry in it, so it is on unless asked otherwise.
func TestIDEDataSourceEnabledByDefault(t *testing.T) {
	writeGlobal(t, "editor: code\n")
	if !IDEDataSourceEnabled() {
		t.Error("want enabled when the key is absent")
	}
}

func TestIDEDataSourceCanBeTurnedOff(t *testing.T) {
	writeGlobal(t, "ide_data_source: false\n")
	if IDEDataSourceEnabled() {
		t.Error("want disabled when the key is false")
	}
	writeGlobal(t, "ide_data_source: true\n")
	if !IDEDataSourceEnabled() {
		t.Error("want enabled when the key is true")
	}
}
