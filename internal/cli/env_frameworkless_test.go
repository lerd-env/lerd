package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// A project with no framework has no env mapping to write. `lerd env` there
// fails with "no framework detected", and every caller that sweeps sites (a
// service port move, the runtime switch) printed that failure as a warning on
// each run.
func TestEnvSkippedForAFrameworklessProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("domains:\n  - proxyapp\nproxy:\n  port: 3000\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ran := false
	runEnvIfManaged(dir, func() error {
		ran = true
		return nil
	})
	if ran {
		t.Error("a project with no framework must not be sent through lerd env")
	}
}

// A project whose .lerd.yaml names a framework still runs, even if nothing on
// disk is detectable from the temp dir.
func TestEnvRunsForADeclaredFramework(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("domains:\n  - shop\nframework: laravel\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ran := false
	runEnvIfManaged(dir, func() error {
		ran = true
		return nil
	})
	if !ran {
		t.Error("a declared framework must still run lerd env")
	}
}
