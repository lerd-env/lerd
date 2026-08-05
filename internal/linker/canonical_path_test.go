package linker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestFreeSiteName_reusesNameForSymlinkedSpelling(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	appReal := filepath.Join(real, "app")
	appLink := filepath.Join(link, "app")
	if err := os.MkdirAll(appReal, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: appReal}); err != nil {
		t.Fatal(err)
	}

	// Linking the same directory through the symlinked spelling must reuse the
	// existing name, not disambiguate it to app-2 (#930).
	if got := FreeSiteName("app", appLink); got != "app" {
		t.Errorf("FreeSiteName(app, symlinked spelling) = %q, want app", got)
	}
}

// One directory is one site. Linking an already-registered directory under a
// different name used to add a second entry for the same path, which is how a
// project ends up served twice with duplicate vhosts and duplicate workers.
func TestResolve_refusesASecondNameForAnAlreadyLinkedPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	if err := config.AddSite(config.Site{Name: "app", Path: dir}); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve(dir, testConfig(), CLIPolicy("other", false, nil))
	if err == nil {
		t.Fatal("linking an already-linked directory under a new name was allowed")
	}
	if !strings.Contains(err.Error(), "app") {
		t.Errorf("error should name the existing site, got: %v", err)
	}

	// Re-linking under the same name stays a re-link, not an error.
	if _, err := Resolve(dir, testConfig(), CLIPolicy("app", false, nil)); err != nil {
		t.Errorf("re-linking under the same name failed: %v", err)
	}
}
