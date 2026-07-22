package phpini

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestValid_SharedScope(t *testing.T) {
	if !Valid(SharedScope) {
		t.Errorf("Valid(%q) = false, want true", SharedScope)
	}
}

func TestScopeFile_SharedMapsToSharedIni(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	f := ScopeFile(SharedScope)
	if f.Path != config.SharedIniFile() {
		t.Errorf("shared scope path = %q, want %q", f.Path, config.SharedIniFile())
	}
	if filepath.Base(f.Path) != "95-shared.ini" {
		t.Errorf("shared scope basename = %q, want 95-shared.ini", filepath.Base(f.Path))
	}
	if !strings.Contains(f.Template, "applied to every PHP version") {
		t.Errorf("shared template missing its guidance:\n%s", f.Template)
	}
}

func TestScopeFile_VersionMapsToPerVersionIni(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	f := ScopeFile("8.4")
	if f.Path != config.PHPUserIniFile("8.4") {
		t.Errorf("version scope path = %q, want %q", f.Path, config.PHPUserIniFile("8.4"))
	}
}

// A reset deletes the shared file, so RestartNoSeed must re-create it before the
// containers restart — every PHP container mounts it and a missing bind-mount
// source makes them all fail to start on podman that does not auto-create it.
func TestRestartNoSeed_SharedReseedsFile(t *testing.T) {
	// Pin the version list empty so the restart loop is a no-op here and the
	// assertion is about the reseed alone. Left live it would iterate whatever
	// the developer has installed and drive their real podman.
	restore := installedVersions
	installedVersions = func() []string { return nil }
	t.Cleanup(func() { installedVersions = restore })

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// Simulate the post-reset state: no shared file on disk.
	if err := RestartNoSeed(SharedScope); err != nil {
		t.Fatalf("RestartNoSeed(shared): %v", err)
	}
	got, err := ScopeFile(SharedScope).Read()
	if err != nil {
		t.Fatalf("reading shared ini: %v", err)
	}
	if !got.Exists {
		t.Errorf("shared ini must be re-seeded after a reset, but it is missing")
	}
}

func TestEnsure_SharedCreatesFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := Ensure(SharedScope); err != nil {
		t.Fatalf("Ensure(shared): %v", err)
	}
	got, err := ScopeFile(SharedScope).Read()
	if err != nil {
		t.Fatalf("reading seeded shared ini: %v", err)
	}
	if !got.Exists {
		t.Errorf("shared ini not created on disk by Ensure")
	}
}
