package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// bunFPMContainer must target a custom-FPM site's per-site container when run from
// inside it (so a custom-FPM-only user can reach the shared bun volume), and the
// shared per-version container everywhere else.
func TestBunFPMContainer_CustomFPMSite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	siteDir := filepath.Join(home, "shop")
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{
		{Name: "shop", Path: siteDir, PHPVersion: "8.4", Runtime: "fpm-custom"},
	}}); err != nil {
		t.Fatalf("seed sites: %v", err)
	}

	t.Chdir(siteDir)
	if got, want := bunFPMContainer("8.4"), podman.CustomFPMContainerName("shop"); got != want {
		t.Errorf("inside custom-FPM site: bunFPMContainer = %q, want %q", got, want)
	}

	t.Chdir(home)
	if got, want := bunFPMContainer("8.4"), fpmContainerName("8.4"); got != want {
		t.Errorf("outside any custom-FPM site: bunFPMContainer = %q, want %q", got, want)
	}
}

// bunDisplayVersion must report a custom-FPM site's pinned version (fixed by its
// Containerfile FROM line) rather than what the project would detect, so php:bun's
// messages don't show a version that differs from the container it actually uses.
func TestBunDisplayVersion_CustomFPMUsesPinned(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	siteDir := filepath.Join(home, "shop")
	if err := os.MkdirAll(siteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// .php-version says 8.3, but the site is pinned to 8.4 by its FROM line.
	if err := os.WriteFile(filepath.Join(siteDir, ".php-version"), []byte("8.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{
		{Name: "shop", Path: siteDir, PHPVersion: "8.4", Runtime: "fpm-custom"},
	}}); err != nil {
		t.Fatalf("seed sites: %v", err)
	}

	t.Chdir(siteDir)
	got, err := bunDisplayVersion(nil)
	if err != nil {
		t.Fatalf("bunDisplayVersion: %v", err)
	}
	if got != "8.4" {
		t.Errorf("inside custom-FPM site: bunDisplayVersion = %q, want 8.4 (pinned, not detected 8.3)", got)
	}
}

// removeContainerBun should clear every entry in the shared bun volume so a
// later install starts clean, while leaving the mount dir itself in place.
func TestRemoveContainerBun_ClearsVolume(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	dir := podman.BunVolumeDir()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "bun"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := removeContainerBun(&out); err != nil {
		t.Fatalf("removeContainerBun: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("bun volume dir should still exist: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("bun volume should be empty after remove, has %d entries", len(entries))
	}
	if !strings.Contains(out.String(), "Removed the in-container bun") {
		t.Errorf("unexpected output: %q", out.String())
	}
}

// On a volume that was never populated, remove is a no-op that says so rather
// than erroring, so reruns and never-installed states stay friendly.
func TestRemoveContainerBun_NothingInstalled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	var out bytes.Buffer
	if err := removeContainerBun(&out); err != nil {
		t.Fatalf("removeContainerBun: %v", err)
	}
	if !strings.Contains(out.String(), "nothing to remove") {
		t.Errorf("expected nothing-to-remove message, got: %q", out.String())
	}
}
