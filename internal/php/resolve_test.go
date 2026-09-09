package php

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// tempRoot is t.TempDir() with symlinks resolved, so a path spelled the way the
// registry stores it also matches by prefix. macOS hands out /var/folders
// tempdirs, and /var is a symlink to /private/var there.
func tempRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// A site served from its own image must be reached by every exec path, not just
// nginx: resolving from the version alone lands in the shared container, which
// has none of the site's custom layers (#1660).
func TestFPMContainerForDir_CustomFPMSite(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	site := filepath.Join(tempRoot(t), "app")
	sub := filepath.Join(site, "app", "Console")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.4", Runtime: "fpm-custom"}); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{site, sub} {
		if got, want := FPMContainerForDir(dir, "8.4"), "lerd-cfpm-app"; got != want {
			t.Errorf("FPMContainerForDir(%q) = %q, want %q", dir, got, want)
		}
	}
}

// A port-bearing custom site serves from its own container too. PHP tooling
// invoked from the project must not fall back to the shared FPM image.
func TestFPMContainerForDir_CustomContainerSite(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	site := filepath.Join(tempRoot(t), "app")
	if err := os.MkdirAll(site, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: site, ContainerPort: 8474}); err != nil {
		t.Fatal(err)
	}

	if got, want := FPMContainerForDir(site, "8.5"), "lerd-custom-app"; got != want {
		t.Errorf("FPMContainerForDir = %q, want %q", got, want)
	}
}

// An ordinary site stays on the shared per-version container.
func TestFPMContainerForDir_SharedFPMSite(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	site := filepath.Join(tempRoot(t), "app")
	if err := os.MkdirAll(site, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.4"}); err != nil {
		t.Fatal(err)
	}

	if got, want := FPMContainerForDir(site, "8.4"), "lerd-php84-fpm"; got != want {
		t.Errorf("FPMContainerForDir = %q, want %q", got, want)
	}
}

// A directory that belongs to no site has only the shared container to run in.
func TestFPMContainerForDir_UnregisteredDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if got, want := FPMContainerForDir(tempRoot(t), "8.3"), "lerd-php83-fpm"; got != want {
		t.Errorf("FPMContainerForDir = %q, want %q", got, want)
	}
}
