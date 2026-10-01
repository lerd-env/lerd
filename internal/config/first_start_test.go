package config

import (
	"os"
	"testing"
)

// Only a directory lerd creates gets a pending first start; one that is
// already there may hold a user's data and is never marked.
func TestEnsureServiceDataDir_MarksOnlyANewDirectory(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if err := EnsureServiceDataDir("mysql"); err != nil {
		t.Fatal(err)
	}
	if !FirstStartPending("mysql") {
		t.Error("a directory lerd just created was not marked pending")
	}

	ClearFirstStartPending("mysql")
	if err := EnsureServiceDataDir("mysql"); err != nil {
		t.Fatal(err)
	}
	if FirstStartPending("mysql") {
		t.Error("an existing directory was marked pending")
	}

	if err := os.MkdirAll(DataSubDir("redis"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureServiceDataDir("redis"); err != nil {
		t.Fatal(err)
	}
	if FirstStartPending("redis") {
		t.Error("a directory that existed before was marked pending")
	}
}
