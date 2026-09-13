package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// symfonyProject writes a .lerd.yaml carrying its own framework definition, so
// the toggle reads the declared paths without reaching for the store.
func symfonyProject(t *testing.T, declares bool) string {
	t.Helper()
	// The store lookup follows HOME and the XDG vars, so without staging both a
	// definition installed on the developer's machine answers instead.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	dir := t.TempDir()
	body := "framework: symfony\nframework_def:\n  name: symfony\n"
	if declares {
		body += "  tmpfs_paths:\n    - var/cache\n"
	}
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSetCacheInMemory_PersistsTheOptIn(t *testing.T) {
	dir := symfonyProject(t, true)
	if err := setCacheInMemory(dir, true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	proj, err := config.LoadProjectConfig(dir)
	if err != nil || !proj.CacheInMemory {
		t.Fatalf("cache_in_memory not persisted: %+v (%v)", proj, err)
	}
	// A macOS-only filesystem choice must not travel to a teammate through git.
	committed, _ := os.ReadFile(filepath.Join(dir, ".lerd.yaml"))
	if strings.Contains(string(committed), "cache_in_memory") {
		t.Errorf("the opt-in leaked into the committed .lerd.yaml:\n%s", committed)
	}
	if _, err := os.Stat(config.LocalOverridePath(dir)); err != nil {
		t.Errorf("the opt-in should live in %s: %v", config.LocalOverrideFile, err)
	}
	if err := setCacheInMemory(dir, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if proj, _ := config.LoadProjectConfig(dir); proj.CacheInMemory {
		t.Error("cache_in_memory should be off again")
	}
}

// Turning it on for a framework that names no path would mount nothing and
// silently do nothing, so it is refused at the input instead.
func TestSetCacheInMemory_RefusesAFrameworkThatDeclaresNoPath(t *testing.T) {
	err := setCacheInMemory(symfonyProject(t, false), true)
	if err == nil || !strings.Contains(err.Error(), "declares no") {
		t.Fatalf("want a refusal naming the missing declaration, got %v", err)
	}
}

// Switching it off must work regardless, so a project that moved to a framework
// version without the declaration can still undo the opt-in.
func TestSetCacheInMemory_TurnsOffWithoutTheDeclaration(t *testing.T) {
	dir := symfonyProject(t, false)
	if err := setCacheInMemory(dir, false); err != nil {
		t.Fatalf("disable should never be refused: %v", err)
	}
}
