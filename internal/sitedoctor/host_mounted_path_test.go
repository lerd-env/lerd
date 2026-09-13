package sitedoctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// cacheDirSite returns a project directory carrying a var/cache tree.
func cacheDirSite(t *testing.T) string {
	t.Helper()
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
