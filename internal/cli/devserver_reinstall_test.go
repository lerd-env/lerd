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
// vite, which keeps serving stale bundles. Once the install lands it is restarted;
// a container worker is left alone, and nothing repeats without a new install.
func TestDevServerInstallsRestartsADevServerAfterAReinstall(t *testing.T) {
	_, site, fake, marker := reinstallSite(t)
	installs := NewDevServerInstalls()

	if err := installs.Check(site); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("the first look only records the install; calls: %v", fake.calls)
	}

	touchLater(t, marker)
	if err := installs.Check(site); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fake.calls, []string{"restart lerd-vite-myapp"}) {
		t.Errorf("want only vite restarted; calls: %v", fake.calls)
	}

	if err := installs.Check(site); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 {
		t.Errorf("restarted again with no new install; calls: %v", fake.calls)
	}
}

// Yarn 1 writes no install-state file, only its integrity file in node_modules.
func TestDevServerInstallsSeesAYarnClassicInstall(t *testing.T) {
	dir, site, fake, marker := reinstallSite(t)
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "yarn.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	integrity := filepath.Join(dir, "node_modules", ".yarn-integrity")
	if err := os.WriteFile(integrity, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	installs := NewDevServerInstalls()
	if err := installs.Check(site); err != nil {
		t.Fatal(err)
	}
	touchLater(t, integrity)
	if err := installs.Check(site); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(fake.calls, "restart lerd-vite-myapp") {
		t.Errorf("a yarn 1 install went unnoticed; calls: %v", fake.calls)
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

// npm ci deletes the config a dev server's unit starts it with, so every start
// writes it back first; without that, the next restart fails on a missing file.
func TestRestoreDevServerFilesWritesBackTheDeletedConfig(t *testing.T) {
	dir, _, _, _ := reinstallSite(t)
	if err := os.RemoveAll(filepath.Join(dir, "node_modules", ".lerd")); err != nil {
		t.Fatal(err)
	}
	if err := RestoreDevServerFiles(dir); err != nil {
		t.Fatal(err)
	}
	tool := config.DevServerToolInstalled(dir)
	if _, err := os.Stat(filepath.Join(dir, tool.WrapperPath)); err != nil {
		t.Errorf("generated dev server config not written back: %v", err)
	}
}

func TestRestoreDevServerFilesRefusesAPathNoSiteOwns(t *testing.T) {
	reinstallSite(t)
	if err := RestoreDevServerFiles(t.TempDir()); err == nil {
		t.Error("want an error for a path no site owns")
	}
}
