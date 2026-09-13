package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetLocalOverride_WritesTheKeyToTheUntrackedFile(t *testing.T) {
	dir := t.TempDir()
	if err := SetLocalOverride(dir, "cache_in_memory", true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(LocalOverridePath(dir))
	if err != nil {
		t.Fatalf("local override file not written: %v", err)
	}
	if !strings.Contains(string(data), "cache_in_memory: true") {
		t.Errorf("key not in the local file:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, ".lerd.yaml")); err == nil {
		t.Error("the committed file must not be created for a machine-local choice")
	}
}

// The local file is shared with every other per-machine override, so writing one
// key must leave the rest of it alone.
func TestSetLocalOverride_KeepsTheOtherKeys(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(LocalOverridePath(dir), []byte("php_version: \"8.3\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetLocalOverride(dir, "cache_in_memory", true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(LocalOverridePath(dir))
	for _, want := range []string{"php_version: \"8.3\"", "cache_in_memory: true"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %q in:\n%s", want, data)
		}
	}
}

func TestSetLocalOverride_OverwritesItsOwnKey(t *testing.T) {
	dir := t.TempDir()
	if err := SetLocalOverride(dir, "cache_in_memory", true); err != nil {
		t.Fatal(err)
	}
	if err := SetLocalOverride(dir, "cache_in_memory", false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(LocalOverridePath(dir))
	if strings.Contains(string(data), "true") {
		t.Errorf("the key should have been replaced:\n%s", data)
	}
}

// The whole point of the local file is that the choice is read back without
// reaching the committed one.
func TestSetLocalOverride_IsReadBackByLoadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("framework: symfony\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetLocalOverride(dir, "cache_in_memory", true); err != nil {
		t.Fatal(err)
	}
	proj, err := LoadProjectConfig(dir)
	if err != nil || !proj.CacheInMemory {
		t.Fatalf("local override not read back: %+v (%v)", proj, err)
	}
	committed, _ := os.ReadFile(filepath.Join(dir, ".lerd.yaml"))
	if strings.Contains(string(committed), "cache_in_memory") {
		t.Errorf("the machine-local choice leaked into the committed file:\n%s", committed)
	}
}
