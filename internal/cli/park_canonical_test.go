package cli

import (
	"os"
	"path/filepath"
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
	if got := freeSiteName("app", appLink); got != "app" {
		t.Errorf("freeSiteName(app, symlinked spelling) = %q, want app", got)
	}
}
