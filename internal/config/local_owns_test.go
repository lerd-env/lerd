package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLocalOverride(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, LocalOverrideFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A command that is about to change a key the local override owns has to know
// before it does any work. `lerd isolate 8.5` under a local file pinning 8.3
// used to warn that the value "was not saved" and then pin 8.5 anyway: it wrote
// .php-version, restarted the pool onto 8.5, and lerd which reported 8.5, while
// .lerd.yaml still said 8.3. The next lerd link silently put the site back on
// 8.3, so the version moved twice with no user action in between.
func TestLocalOverrideOwns(t *testing.T) {
	dir := writeLocalOverride(t, "php_version: \"8.3\"\nnode_version: \"20\"\n")

	for _, key := range []string{"php_version", "node_version"} {
		owns, err := LocalOverrideOwns(dir, key)
		if err != nil {
			t.Fatalf("LocalOverrideOwns(%q): %v", key, err)
		}
		if !owns {
			t.Errorf("LocalOverrideOwns(%q) = false, want true", key)
		}
	}

	owns, err := LocalOverrideOwns(dir, "domains")
	if err != nil {
		t.Fatal(err)
	}
	if owns {
		t.Error("LocalOverrideOwns reported ownership of a key the file does not set")
	}
}

// No local file at all is the common case and must not error.
func TestLocalOverrideOwnsWithNoLocalFile(t *testing.T) {
	owns, err := LocalOverrideOwns(t.TempDir(), "php_version")
	if err != nil {
		t.Fatalf("unexpected error with no local file: %v", err)
	}
	if owns {
		t.Error("reported ownership with no local override file present")
	}
}

// The refusal a command shows has to name the file, the key and the way out, so
// it reads as the answer rather than as a warning about something already done.
func TestLocalOverrideRefusal(t *testing.T) {
	err := LocalOverrideRefusal("php_version")
	if err == nil {
		t.Fatal("LocalOverrideRefusal returned nil")
	}
	for _, want := range []string{LocalOverrideFile, "php_version"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not mention %q:\n%s", want, err.Error())
		}
	}
}
