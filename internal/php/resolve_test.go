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

// stubCustomImagePHP answers the image probe without building an image.
func stubCustomImagePHP(t *testing.T, has bool) {
	t.Helper()
	orig := customImageHasPHPFn
	t.Cleanup(func() { customImageHasPHPFn = orig })
	customImageHasPHPFn = func(string) bool { return has }
}

// A port-bearing custom site whose image carries PHP serves its tooling from
// its own container, so a project runtime is not silently replaced by the
// shared one.
func TestFPMContainerForDir_CustomContainerSite(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubCustomImagePHP(t, true)

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

// The section exists for Node, Python and Go sites as much as for PHP ones, and
// execing php into an image without it fails at the OCI runtime. Those sites
// keep the shared container, where the project is visible and php exists.
func TestFPMContainerForDir_CustomContainerWithoutPHP(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubCustomImagePHP(t, false)

	site := filepath.Join(tempRoot(t), "node-app")
	if err := os.MkdirAll(site, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "node-app", Path: site, ContainerPort: 3000}); err != nil {
		t.Fatal(err)
	}

	if got, want := FPMContainerForDir(site, "8.5"), "lerd-php85-fpm"; got != want {
		t.Errorf("FPMContainerForDir = %q, want %q", got, want)
	}
}

// A shell under /home on an ostree host is inside the project registered under
// /var/home, so a command run from one of its folders finds the site root.
func TestSiteRootForFromTheOtherHomeSpelling(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := tempRoot(t)
	site := filepath.Join(root, "var-home", "u", "app")
	if err := os.MkdirAll(filepath.Join(site, "database"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "var-home"), filepath.Join(root, "home")); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.4"}); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(root, "home", "u", "app", "database")
	if got := SiteRootFor(dir); got != site {
		t.Errorf("SiteRootFor(%q) = %q, want the registered %q", dir, got, site)
	}
}

// Under the native runtime the shared FPM container is stopped and its image
// may be gone, so a FrankenPHP site's commands go to the site's own container.
// Container mode keeps the shared one, as it always has.
func TestFPMContainerForDir_FrankenPHPSite(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	site := filepath.Join(tempRoot(t), "shop")
	if err := os.MkdirAll(site, 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Path: site, PHPVersion: "8.5", Runtime: "frankenphp"}); err != nil {
		t.Fatal(err)
	}

	stubRuntimeInstalled(t, true, nil, nil)
	if got, want := FPMContainerForDir(site, "8.5"), "lerd-fp-shop"; got != want {
		t.Errorf("native: FPMContainerForDir = %q, want %q", got, want)
	}
	stubRuntimeInstalled(t, false, nil, nil)
	if got, want := FPMContainerForDir(site, "8.5"), "lerd-php85-fpm"; got != want {
		t.Errorf("container: FPMContainerForDir = %q, want %q", got, want)
	}
}

// Only a site whose PHP has a container of its own leaves the host under the
// native runtime. A host-proxy site and a custom container without PHP have
// nowhere else to run it, and a plain site is what the host serves.
func TestRunsInOwnContainer(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubRuntimeInstalled(t, true, nil, nil)
	root := tempRoot(t)
	cases := []struct {
		site     config.Site
		imagePHP bool
		want     bool
	}{
		{config.Site{Name: "franken", Runtime: "frankenphp"}, false, true},
		{config.Site{Name: "cfpm", Runtime: "fpm-custom"}, false, true},
		{config.Site{Name: "custom", ContainerPort: 8474}, true, true},
		{config.Site{Name: "node", ContainerPort: 3000}, false, false},
		{config.Site{Name: "proxy", HostPort: 5173}, false, false},
		{config.Site{Name: "plain"}, false, false},
	}
	for _, c := range cases {
		c.site.Path = filepath.Join(root, c.site.Name)
		c.site.PHPVersion = "8.5"
		if err := os.MkdirAll(c.site.Path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := config.AddSite(c.site); err != nil {
			t.Fatal(err)
		}
		stubCustomImagePHP(t, c.imagePHP)
		if got := RunsInOwnContainer(c.site.Path); got != c.want {
			t.Errorf("%s: RunsInOwnContainer = %v, want %v", c.site.Name, got, c.want)
		}
	}
	if RunsInOwnContainer(filepath.Join(root, "elsewhere")) {
		t.Error("a folder that is no site keeps running on the host")
	}
}
