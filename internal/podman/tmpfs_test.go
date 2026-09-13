package podman

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

const fpmUnit = `[Container]
Image=lerd-php85-fpm:local
ContainerName=lerd-php85-fpm
Volume=%h:%h:rw
Volume=/opt/work:/opt/work:rw
`

func TestInjectTmpfs_AddsALineForEachPath(t *testing.T) {
	out := InjectTmpfs(fpmUnit, []string{"/home/u/site/var/cache", "/opt/work/app/var/cache"})
	for _, want := range []string{"Tmpfs=/home/u/site/var/cache", "Tmpfs=/opt/work/app/var/cache"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// A tmpfs must land after the bind mount it shadows, or podman layers the mount
// over the tmpfs and the cache goes straight back onto virtiofs.
func TestInjectTmpfs_LandsAfterTheHomeBindMount(t *testing.T) {
	out := InjectTmpfs(fpmUnit, []string{"/home/u/site/var/cache"})
	if strings.Index(out, "Tmpfs=/home/u/site/var/cache") < strings.Index(out, "Volume=%h:%h:rw") {
		t.Errorf("tmpfs must come after the home mount:\n%s", out)
	}
}

func TestInjectTmpfs_SkipsWhatIsAlreadyThere(t *testing.T) {
	once := InjectTmpfs(fpmUnit, []string{"/home/u/site/var/cache"})
	twice := InjectTmpfs(once, []string{"/home/u/site/var/cache"})
	if once != twice {
		t.Errorf("a second pass must not duplicate the line:\n%s", twice)
	}
}

func TestInjectTmpfs_RefusesAPathThatIsNotAbsolute(t *testing.T) {
	out := InjectTmpfs(fpmUnit, []string{"var/cache", "", "/"})
	if strings.Contains(out, "Tmpfs=") {
		t.Errorf("only absolute, non-root paths may be mounted:\n%s", out)
	}
}

func TestInjectTmpfs_LeavesTheUnitAloneWithNoPaths(t *testing.T) {
	if out := InjectTmpfs(fpmUnit, nil); out != fpmUnit {
		t.Errorf("unit changed with no paths:\n%s", out)
	}
}

// declaredCache stands in for the store lookup: the site's framework says which
// directories are worth holding in memory, and the project has opted in.
func declaredCache(_ config.Site) []string { return []string{"var/cache"} }

func siteOn(version, path string) config.Site {
	return config.Site{Name: filepath.Base(path), Path: path, PHPVersion: version}
}

func TestTmpfsPaths_ResolvesTheDeclaredPathsAgainstEachSiteRoot(t *testing.T) {
	sites := []config.Site{siteOn("8.5", "/home/u/symf"), siteOn("8.4", "/home/u/old")}
	got := tmpfsPaths(sites, "8.5", declaredCache)
	want := []string{"/home/u/symf/var/cache"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTmpfsPaths_SkipsASiteThatDeclaresNothing(t *testing.T) {
	sites := []config.Site{siteOn("8.5", "/home/u/plain")}
	if got := tmpfsPaths(sites, "8.5", func(config.Site) []string { return nil }); got != nil {
		t.Errorf("nothing declared, so nothing to mount, got %v", got)
	}
}

// The shared FPM container serves only the sites that run on it. A site with its
// own runtime has no line in this unit to add.
func TestTmpfsPaths_SkipsASiteOffTheSharedContainer(t *testing.T) {
	franken := siteOn("8.5", "/home/u/franken")
	franken.Runtime = "frankenphp"
	if got := tmpfsPaths([]config.Site{franken}, "8.5", declaredCache); got != nil {
		t.Errorf("a site off the shared FPM container has nothing here, got %v", got)
	}
}

// projectWithCache writes a .lerd.yaml carrying its own framework definition, so
// the resolver reads the declared paths without reaching for the store.
func projectWithCache(t *testing.T, optIn bool) string {
	t.Helper()
	dir := t.TempDir()
	body := "framework: symfony\nframework_def:\n  name: symfony\n  tmpfs_paths:\n    - var/cache\n"
	if optIn {
		body += "cache_in_memory: true\n"
	}
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDeclaredTmpfsPaths_ReadsTheFrameworkPathsWhenTheProjectOptsIn(t *testing.T) {
	dir := projectWithCache(t, true)
	got := declaredTmpfsPaths(config.Site{Path: dir})
	if !reflect.DeepEqual(got, []string{"var/cache"}) {
		t.Errorf("got %v, want [var/cache]", got)
	}
}

// Opting in belongs to the project, not the framework: a Symfony site that never
// asked keeps its cache on disk where the host can read it.
func TestDeclaredTmpfsPaths_SilentWithoutTheOptIn(t *testing.T) {
	dir := projectWithCache(t, false)
	if got := declaredTmpfsPaths(config.Site{Path: dir}); got != nil {
		t.Errorf("the project has not opted in, got %v", got)
	}
}

// Linux shares a filesystem with PHP and the native runtime runs PHP on the
// host, so neither has a mount to escape.
func TestTmpfsSupported_OnlyMacOSOnTheContainerRuntime(t *testing.T) {
	cases := []struct {
		goos   string
		native bool
		want   bool
	}{
		{"darwin", false, true},
		{"darwin", true, false},
		{"linux", false, false},
	}
	for _, c := range cases {
		if got := tmpfsSupported(c.goos, c.native); got != c.want {
			t.Errorf("tmpfsSupported(%q, %v) = %v, want %v", c.goos, c.native, got, c.want)
		}
	}
}
