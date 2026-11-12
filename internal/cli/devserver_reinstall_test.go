package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

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

// reinstallSite is devServerSite with vite and queue running, an npm install
// marker in place, and a HOME of its own, since a worker start on macOS writes
// its plist there.
func reinstallSite(t *testing.T) (string, *config.Site, *statefulLifecycle, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir := devServerSite(t, false)
	site, err := config.FindSite("myapp")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "node_modules", ".package-lock.json")
	if err := os.WriteFile(marker, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &statefulLifecycle{active: map[string]bool{"lerd-vite-myapp": true, "lerd-queue-myapp": true}}
	podman.UnitLifecycle = fake
	t.Cleanup(func() { podman.UnitLifecycle = nil })
	return dir, site, fake, marker
}

func touchLater(t *testing.T, path string) {
	t.Helper()
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

// npm ci, from the dashboard or a terminal, replaces node_modules under a running
// vite, which keeps stale bundles and loses its config. Once the install lands it
// is restarted with that config written back; a container worker is left alone.
func TestDevServerInstallsRestartsADevServerAfterAReinstall(t *testing.T) {
	dir, site, fake, marker := reinstallSite(t)
	installs := NewDevServerInstalls()

	if err := installs.Check(site); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("the first look only records the install; calls: %v", fake.calls)
	}

	if err := os.RemoveAll(filepath.Join(dir, "node_modules", ".lerd")); err != nil {
		t.Fatal(err)
	}
	touchLater(t, marker)
	if err := installs.Check(site); err != nil {
		t.Fatal(err)
	}

	stop := slices.Index(fake.calls, "stop lerd-vite-myapp")
	start := slices.Index(fake.calls, "start lerd-vite-myapp")
	if stop < 0 || start < stop {
		t.Errorf("want vite stopped and started again; calls: %v", fake.calls)
	}
	if slices.ContainsFunc(fake.calls, func(c string) bool { return c == "stop lerd-queue-myapp" || c == "restart lerd-queue-myapp" }) {
		t.Errorf("the container worker was touched; calls: %v", fake.calls)
	}
	tool := config.DevServerToolInstalled(dir)
	if _, err := os.Stat(filepath.Join(dir, tool.WrapperPath)); err != nil {
		t.Errorf("generated dev server config missing after the restart: %v", err)
	}

	before := len(fake.calls)
	if err := installs.Check(site); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != before {
		t.Errorf("restarted again with no new install; calls: %v", fake.calls[before:])
	}
}

// Mid-install the marker is gone: nothing is restarted until the install lands.
func TestDevServerInstallsWaitsWhileTheInstallRuns(t *testing.T) {
	_, site, fake, marker := reinstallSite(t)
	installs := NewDevServerInstalls()
	if err := installs.Check(site); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := installs.Check(site); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("restarted while the install was still running; calls: %v", fake.calls)
	}
}

// A dev server that is not running is not started by an install.
func TestDevServerInstallsLeavesAStoppedDevServerStopped(t *testing.T) {
	_, site, fake, marker := reinstallSite(t)
	fake.active["lerd-vite-myapp"] = false
	installs := NewDevServerInstalls()
	if err := installs.Check(site); err != nil {
		t.Fatal(err)
	}
	touchLater(t, marker)
	if err := installs.Check(site); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("a stopped dev server was touched; calls: %v", fake.calls)
	}
}
