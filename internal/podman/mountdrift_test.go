package podman

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// fakeInspect stubs `podman inspect` with one canned line, the format
// containerMounts asks for.
func fakeInspect(t *testing.T, line string) {
	t.Helper()
	prev := execCommand
	t.Cleanup(func() { execCommand = prev })
	execCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("printf", "%s", line)
	}
}

type restartRecorder struct{ restarted []string }

func (r *restartRecorder) Start(string) error { return nil }
func (r *restartRecorder) Stop(string) error  { return nil }
func (r *restartRecorder) Restart(name string) error {
	r.restarted = append(r.restarted, name)
	return nil
}
func (r *restartRecorder) UnitStatus(string) (string, error) { return "active", nil }
func (r *restartRecorder) AllUnitStates() map[string]string  { return nil }

func TestUnitMissingMounts(t *testing.T) {
	fakeInspect(t, "true#/home/george|/srv/apps|")

	if UnitMissingMounts("lerd-php84-fpm", []string{"/srv/apps/shop"}) {
		t.Error("a path under an existing mount source is not missing")
	}
	if UnitMissingMounts("lerd-php84-fpm", []string{"/srv/apps"}) {
		t.Error("the mount source itself is not missing")
	}
	if !UnitMissingMounts("lerd-php84-fpm", []string{"/data/Projects/app"}) {
		t.Error("a path the running container has no mount for is missing")
	}
	if !UnitMissingMounts("lerd-php84-fpm", []string{"/srv/appsuite"}) {
		t.Error("ancestor matching must respect the path separator")
	}
	if UnitMissingMounts("lerd-php84-fpm", nil) {
		t.Error("no paths means nothing is missing")
	}
}

// A stopped container picks the quadlet up when it next starts, so it never
// counts as drifted and must not trigger a restart.
func TestUnitMissingMountsIgnoresStoppedContainer(t *testing.T) {
	fakeInspect(t, "false#")
	if UnitMissingMounts("lerd-php84-fpm", []string{"/data/Projects/app"}) {
		t.Error("a stopped container must not report drift")
	}
}

func TestUnitMissingMountsIgnoresInspectFailure(t *testing.T) {
	prev := execCommand
	t.Cleanup(func() { execCommand = prev })
	execCommand = func(string, ...string) *exec.Cmd { return exec.Command("false") }

	if UnitMissingMounts("lerd-php84-fpm", []string{"/data/Projects/app"}) {
		t.Error("an unknown container must not report drift")
	}
}

// The regression from #914: the quadlet file already carries the Volume line
// (FinishLink wrote it) but the container is still running without it, so
// EnsurePathMounted has to restart even though it changes no file.
func TestEnsurePathMountedRestartsDriftedContainer(t *testing.T) {
	home := t.TempDir()
	cfgHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	resetPathMountAttempts()

	quadlets := filepath.Join(cfgHome, "containers", "systemd")
	if err := os.MkdirAll(quadlets, 0755); err != nil {
		t.Fatal(err)
	}
	fpm := filepath.Join(quadlets, "lerd-php84-fpm.container")
	content := "[Container]\nVolume=%h:%h:rw\nVolume=/data/Projects/app:/data/Projects/app:rw\n"
	if err := os.WriteFile(fpm, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(quadlets, "lerd-nginx.container"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	fakeInspect(t, "true#/home/george|")
	lc := &restartRecorder{}
	prevLC := UnitLifecycle
	UnitLifecycle = lc
	t.Cleanup(func() { UnitLifecycle = prevLC })

	EnsurePathMounted("/data/Projects/app", "8.4")

	if len(lc.restarted) != 2 {
		t.Fatalf("restarted %v, want both lerd-php84-fpm and lerd-nginx", lc.restarted)
	}
	if got, err := os.ReadFile(fpm); err != nil || string(got) != content {
		t.Error("the quadlet file was already correct and must not be rewritten")
	}
}

// FinishLink writes the FPM quadlet and only then calls RewriteFPMQuadlets, so
// the rewrite finds the file already correct. It still has to restart, which is
// the second half of #914: the second call here stands in for that rewrite.
func TestRewriteFPMQuadletsRestartsDriftedContainer(t *testing.T) {
	home := t.TempDir()
	cfgHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	quadlets := filepath.Join(cfgHome, "containers", "systemd")
	if err := os.MkdirAll(quadlets, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(quadlets, "lerd-php84-fpm.container"), []byte("[Container]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sitePath := filepath.Join(t.TempDir(), "shop")
	if err := os.MkdirAll(sitePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{{
		Name: "shop", Path: sitePath, PHPVersion: "8.4",
	}}}); err != nil {
		t.Fatal(err)
	}

	fakeInspect(t, "true#/home/george|")
	lc := &restartRecorder{}
	prevLC := UnitLifecycle
	UnitLifecycle = lc
	t.Cleanup(func() { UnitLifecycle = prevLC })

	if err := RewriteFPMQuadlets(); err != nil { // writes the quadlet
		t.Fatal(err)
	}
	lc.restarted = nil
	if err := RewriteFPMQuadlets(); err != nil { // file already correct, container is not
		t.Fatal(err)
	}
	if len(lc.restarted) == 0 {
		t.Error("a container running without the site mount must be restarted even when the quadlet is unchanged")
	}
}

// A custom-FPM site runs its own container, which EnsurePathMounted used to
// skip entirely, so an out-of-home path never reached it.
func TestEnsurePathMountedCoversCustomFPMSites(t *testing.T) {
	home := t.TempDir()
	cfgHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	resetPathMountAttempts()

	quadlets := filepath.Join(cfgHome, "containers", "systemd")
	if err := os.MkdirAll(quadlets, 0755); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(quadlets, "lerd-cfpm-shop.container")
	if err := os.WriteFile(custom, []byte("[Container]\nVolume=%h:%h:rw\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{{
		Name: "shop", Path: "/data/Projects/shop", PHPVersion: "8.4", Runtime: "fpm-custom",
	}}}); err != nil {
		t.Fatal(err)
	}

	fakeInspect(t, "true#/home/george|")
	lc := &restartRecorder{}
	prevLC := UnitLifecycle
	UnitLifecycle = lc
	t.Cleanup(func() { UnitLifecycle = prevLC })

	EnsurePathMounted("/data/Projects/shop", "8.4")

	got, err := os.ReadFile(custom)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Volume=/data/Projects/shop:/data/Projects/shop:rw"; !strings.Contains(string(got), want) {
		t.Errorf("custom FPM quadlet missing %q:\n%s", want, got)
	}
	if len(lc.restarted) != 1 || lc.restarted[0] != "lerd-cfpm-shop" {
		t.Errorf("restarted %v, want lerd-cfpm-shop", lc.restarted)
	}
}
