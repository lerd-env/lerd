package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestWriteNodeVersionFile_fromCommittedPin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("node_version: \"22\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	proj, err := config.LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeNodeVersionFile(dir, proj)
	data, err := os.ReadFile(filepath.Join(dir, ".node-version"))
	if err != nil || string(data) != "22\n" {
		t.Fatalf(".node-version = %q, %v; want 22", data, err)
	}
}

func TestWriteNodeVersionFile_skipsLocalOnlyPin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.local.yaml"), []byte("node_version: \"20\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	proj, err := config.LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeNodeVersionFile(dir, proj)
	if _, err := os.Stat(filepath.Join(dir, ".node-version")); !os.IsNotExist(err) {
		t.Errorf("a pin only .lerd.local.yaml sets must not write .node-version, stat err = %v", err)
	}
}
