package sitedoctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// cacheDirSite returns a project directory carrying a var/cache tree. It stages
// HOME and the XDG vars too: the framework lookup follows them, so without that
// a definition installed on the developer's machine answers instead.
func cacheDirSite(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "var", "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

var cacheSpec = config.DoctorCheck{
	Name:   "cache_on_bind_mount",
	Type:   "host_mounted_path",
	Paths:  []string{"var/cache"},
	Detail: "override getCacheDir() to read APP_CACHE_DIR",
}

func TestHostMountedPath_WarnsOnAContainerSiteOnMacOS(t *testing.T) {
	c, ok := hostMountedPathOn("darwin", false, cacheDirSite(t), cacheSpec)
	if !ok || c.Status != StatusWarn {
		t.Fatalf("want a warn on the macOS bind mount, got %+v (ok=%v)", c, ok)
	}
	if c.Detail != cacheSpec.Detail {
		t.Errorf("the framework's own remedy should be the detail, got %q", c.Detail)
	}
}

// The penalty is a VM boundary macOS pays alone; on Linux the project already
// shares a filesystem with PHP, so the row would be pure noise.
func TestHostMountedPath_SilentOnLinux(t *testing.T) {
	if _, ok := hostMountedPathOn("linux", false, cacheDirSite(t), cacheSpec); ok {
		t.Error("the check should not report on Linux")
	}
}

// A natively served site runs PHP on the host, so nothing it reads crosses
// virtiofs even though the machine is a Mac.
func TestHostMountedPath_SilentOnANativeSite(t *testing.T) {
	if _, ok := hostMountedPathOn("darwin", true, cacheDirSite(t), cacheSpec); ok {
		t.Error("the check should not report for a site served by the native runtime")
	}
}

func TestHostMountedPath_SilentWhenThePathIsNotThere(t *testing.T) {
	if _, ok := hostMountedPathOn("darwin", false, t.TempDir(), cacheSpec); ok {
		t.Error("a project without the declared path has nothing to report")
	}
}

func TestHostMountedPath_NamesThePathsWhenTheStoreGivesNoDetail(t *testing.T) {
	spec := cacheSpec
	spec.Detail = ""
	c, ok := hostMountedPathOn("darwin", false, cacheDirSite(t), spec)
	if !ok || !strings.Contains(c.Detail, "var/cache") {
		t.Fatalf("the fallback detail should name the path, got %q (ok=%v)", c.Detail, ok)
	}
}

// Once the site holds the cache in memory the directory the check names is a
// tmpfs inside the container, so the warning has nothing left to say.
func TestHostMountedPath_SilentOnceTheSiteHoldsTheCacheInMemory(t *testing.T) {
	dir := cacheDirSite(t)
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("cache_in_memory: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := hostMountedPathOn("darwin", false, dir, cacheSpec); ok {
		t.Error("a site that opted in should not be told to move its cache")
	}
}

// The check is the only place a user meets this problem, so it carries the fix
// that resolves it rather than leaving them to find the command.
func TestHostMountedPath_OffersTheInMemoryCacheFix(t *testing.T) {
	dir := cacheDirSite(t)
	writeLerdYAML(t, dir, "framework: symfony\nframework_def:\n  name: symfony\n  tmpfs_paths:\n    - var/cache\n")
	c, ok := hostMountedPathOn("darwin", false, dir, cacheSpec)
	if !ok {
		t.Fatal("expected the check to report")
	}
	if c.Fix != FixCacheInMemory {
		t.Errorf("Fix = %q, want %q", c.Fix, FixCacheInMemory)
	}
}

// A framework with no declared path has nothing to mount, so offering the button
// would hand the user a fix that cannot work.
func TestHostMountedPath_NoFixWhenTheFrameworkDeclaresNoPath(t *testing.T) {
	dir := cacheDirSite(t)
	writeLerdYAML(t, dir, "framework: symfony\nframework_def:\n  name: symfony\n")
	c, ok := hostMountedPathOn("darwin", false, dir, cacheSpec)
	if !ok {
		t.Fatal("expected the check to report")
	}
	if c.Fix == FixCacheInMemory {
		t.Error("a framework that declares no path must not offer the fix")
	}
}

func writeLerdYAML(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
