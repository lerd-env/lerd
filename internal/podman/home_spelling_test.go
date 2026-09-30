package podman

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// ostreeHome lays out /home → /var/home inside a temp dir and returns the
// home under each spelling.
func ostreeHome(t *testing.T) (realHome, linkHome string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	realRoot := filepath.Join(tmp, "var-home")
	if err := os.MkdirAll(filepath.Join(realRoot, "u", "Projects", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRoot, filepath.Join(tmp, "home")); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(realRoot, "u"), filepath.Join(tmp, "home", "u")
}

// A project reached through the other spelling of home is already inside the
// home mount, so it never earns a Volume line of its own. Those per-path lines
// are what outlived a removed worktree and kept PHP-FPM from starting.
func TestExtraVolumePathsLeavesTheOtherHomeSpellingOut(t *testing.T) {
	realHome, linkHome := ostreeHome(t)
	project := filepath.Join(realHome, "Projects", "app")

	got := extraVolumePaths([]string{project, "/srv/shop"}, linkHome)
	if !slices.Equal(got, []string{"/srv/shop"}) {
		t.Errorf("extraVolumePaths = %v, want only /srv/shop", got)
	}
}

func TestPathVisibleThroughTheOtherHomeSpelling(t *testing.T) {
	realHome, linkHome := ostreeHome(t)
	t.Setenv("HOME", linkHome)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if !PathVisible(filepath.Join(realHome, "Projects", "app"), "8.4") {
		t.Error("a project under the resolved home is inside the home mount")
	}
}

// The unit's %h mounts one spelling; every other one is mounted once, so a
// command run from either spelling finds its folder in the container.
func TestHomeAliases(t *testing.T) {
	spellings := []string{"/home/u", "/var/home/u"}
	if got := homeAliases(spellings, "/home/u"); !slices.Equal(got, []string{"/var/home/u"}) {
		t.Errorf("homeAliases with %%h=/home/u = %v, want [/var/home/u]", got)
	}
	if got := homeAliases(spellings, "/var/home/u"); !slices.Equal(got, []string{"/home/u"}) {
		t.Errorf("homeAliases with %%h=/var/home/u = %v, want [/home/u]", got)
	}
	if got := homeAliases([]string{"/home/u"}, "/home/u"); len(got) != 0 {
		t.Errorf("a plain home has no aliases, got %v", got)
	}
}

// A worktree inside a project that is already mounted is reachable through the
// project's line, so running a command in it must not add a line of its own
// that is left pointing at nothing once the worktree is removed.
func TestEnsurePathMountedSkipsAPathItsAncestorMounts(t *testing.T) {
	home := t.TempDir()
	cfgHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	resetPathMountAttempts()

	quadlets := filepath.Join(cfgHome, "containers", "systemd")
	if err := os.MkdirAll(quadlets, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "[Container]\nVolume=%h:%h:rw\nVolume=/srv/shop:/srv/shop:rw\n"
	for _, name := range []string{"lerd-php84-fpm.container", "lerd-nginx.container"} {
		if err := os.WriteFile(filepath.Join(quadlets, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fakeInspect(t, "true#/srv/shop|")
	lc := &restartRecorder{}
	prevLC := UnitLifecycle
	UnitLifecycle = lc
	t.Cleanup(func() { UnitLifecycle = prevLC })

	EnsurePathMounted("/srv/shop/shop-feat", "8.4")

	got, err := os.ReadFile(filepath.Join(quadlets, "lerd-php84-fpm.container"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "shop-feat") {
		t.Errorf("a worktree under a mounted project got its own line:\n%s", got)
	}
	if len(lc.restarted) != 0 {
		t.Errorf("restarted %v for a path already mounted", lc.restarted)
	}
}

// The alias mount names the same folder as the %h mount, so asking by source,
// which resolves symlinks, finds it already covered on a container started
// before the alias existed. Asked by destination it is missing, and the
// rewrite restarts the container instead of leaving /home paths unreachable.
func TestUnitMissingHomeAliases(t *testing.T) {
	fakeInspect(t, "true#/var/home/u|/etc/hosts|")
	if !unitMissingHomeAliases("lerd-php84-fpm", []string{"/home/u"}) {
		t.Error("a container without the /home alias mount is drifted")
	}

	fakeInspect(t, "true#/var/home/u|/home/u|")
	if unitMissingHomeAliases("lerd-php84-fpm", []string{"/home/u"}) {
		t.Error("a container already carrying the alias is not drifted")
	}

	fakeInspect(t, "false#")
	if unitMissingHomeAliases("lerd-php84-fpm", []string{"/home/u"}) {
		t.Error("a stopped container picks the quadlet up when it starts")
	}
	if unitMissingHomeAliases("lerd-php84-fpm", nil) {
		t.Error("no aliases means nothing to restart for")
	}
}
