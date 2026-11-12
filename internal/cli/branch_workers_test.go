package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// statefulLifecycle tracks which units run, and logs each call in order.
type statefulLifecycle struct {
	active map[string]bool
	calls  []string
}

func (f *statefulLifecycle) Start(name string) error {
	f.active[name] = true
	f.calls = append(f.calls, "start "+name)
	return nil
}
func (f *statefulLifecycle) Stop(name string) error {
	f.active[name] = false
	f.calls = append(f.calls, "stop "+name)
	return nil
}
func (f *statefulLifecycle) Restart(name string) error {
	f.calls = append(f.calls, "restart "+name)
	return nil
}
func (f *statefulLifecycle) UnitStatus(name string) (string, error) {
	if f.active[name] {
		return "active", nil
	}
	return "inactive", nil
}
func (f *statefulLifecycle) AllUnitStates() map[string]string { return nil }

func installTestSite(t *testing.T) (string, *config.Site, *statefulLifecycle) {
	t.Helper()
	// A worker start on macOS writes its plist under HOME.
	t.Setenv("HOME", t.TempDir())
	dir := devServerSite(t, false)
	site, err := config.FindSite("myapp")
	if err != nil {
		t.Fatal(err)
	}
	fake := &statefulLifecycle{active: map[string]bool{"lerd-vite-myapp": true, "lerd-queue-myapp": true}}
	podman.UnitLifecycle = fake
	t.Cleanup(func() { podman.UnitLifecycle = nil })
	return dir, site, fake
}

// npm ci replaces node_modules: a vite left running serves stale bundles and
// loses the config its unit starts it with, so its next restart fails. It has
// to be restarted after the install, with that config written back first.
func TestRestartAfterJSInstallBringsBackTheDevServerConfig(t *testing.T) {
	dir, site, fake := installTestSite(t)
	fw, _ := config.GetFrameworkForDir(site.Framework, dir)

	running := runningHostWorkers(site, fw, dir)
	if !slices.Equal(running, []string{"vite"}) {
		t.Fatalf("running host workers = %v, want only vite: queue runs in a container", running)
	}
	if err := os.RemoveAll(filepath.Join(dir, "node_modules", ".lerd")); err != nil {
		t.Fatal(err)
	}

	if err := restartAfterJSInstall(site, fw, dir, running); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if !slices.Contains(fake.calls, "restart lerd-vite-myapp") {
		t.Errorf("vite was not restarted; calls: %v", fake.calls)
	}
	if slices.ContainsFunc(fake.calls, func(c string) bool { return c == "restart lerd-queue-myapp" || c == "stop lerd-queue-myapp" }) {
		t.Errorf("the container worker was touched; calls: %v", fake.calls)
	}
	tool := config.DevServerToolInstalled(dir)
	if _, err := os.Stat(filepath.Join(dir, tool.WrapperPath)); err != nil {
		t.Errorf("generated dev server config missing after the restart: %v", err)
	}
}

// A worker the new code no longer declares still ran on the old packages.
func TestRestartAfterJSInstallRestartsAWorkerTheNewCodeDropped(t *testing.T) {
	dir, site, fake := installTestSite(t)

	if err := restartAfterJSInstall(site, &config.Framework{Name: "testfw"}, dir, []string{"vite"}); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if !slices.Contains(fake.calls, "restart lerd-vite-myapp") {
		t.Errorf("vite was not restarted; calls: %v", fake.calls)
	}
}
