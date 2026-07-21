package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// fakeUnitLifecycle records which unit was restarted.
type fakeUnitLifecycle struct {
	restartedUnit string
}

func (f *fakeUnitLifecycle) Start(name string) error                { return nil }
func (f *fakeUnitLifecycle) Stop(name string) error                 { return nil }
func (f *fakeUnitLifecycle) Restart(name string) error              { f.restartedUnit = name; return nil }
func (f *fakeUnitLifecycle) UnitStatus(name string) (string, error) { return "active", nil }
func (f *fakeUnitLifecycle) AllUnitStates() map[string]string       { return nil }

func TestRestartSite_CustomContainer(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	siteDir := t.TempDir()
	config.AddSite(config.Site{
		Name:          "nestapp",
		Domains:       []string{"nestapp.test"},
		Path:          siteDir,
		ContainerPort: 3000,
	})

	fake := &fakeUnitLifecycle{}
	podman.UnitLifecycle = fake
	defer func() { podman.UnitLifecycle = nil }()

	if err := RestartSite("nestapp"); err != nil {
		t.Fatalf("RestartSite: %v", err)
	}
	if fake.restartedUnit != "lerd-custom-nestapp" {
		t.Errorf("restarted unit = %q, want lerd-custom-nestapp", fake.restartedUnit)
	}
}

func TestRestartSite_PHPSite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	siteDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(siteDir, "composer.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	config.AddSite(config.Site{
		Name:       "phpapp",
		Domains:    []string{"phpapp.test"},
		Path:       siteDir,
		PHPVersion: "8.4",
	})

	fake := &fakeUnitLifecycle{}
	podman.UnitLifecycle = fake
	defer func() { podman.UnitLifecycle = nil }()

	if err := RestartSite("phpapp"); err != nil {
		t.Fatalf("RestartSite: %v", err)
	}
	if fake.restartedUnit != "lerd-php84-fpm" {
		t.Errorf("restarted unit = %q, want lerd-php84-fpm", fake.restartedUnit)
	}
}

func TestRestartSite_StaticSiteRefused(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	// A static site: a public dir of HTML with no composer.json or .php. It is
	// served directly by nginx and has no per-site container, so restart must
	// refuse rather than bounce the shared FPM container.
	siteDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(siteDir, "public"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siteDir, "public", "index.html"), []byte("<h1>hi</h1>"), 0644); err != nil {
		t.Fatal(err)
	}
	config.AddSite(config.Site{
		Name:       "static",
		Domains:    []string{"static.test"},
		Path:       siteDir,
		PublicDir:  "public",
		PHPVersion: "8.4",
	})

	fake := &fakeUnitLifecycle{}
	podman.UnitLifecycle = fake
	defer func() { podman.UnitLifecycle = nil }()

	if err := RestartSite("static"); err == nil {
		t.Fatal("expected error restarting a static site")
	}
	if fake.restartedUnit != "" {
		t.Errorf("restarted unit = %q, want none for a static site", fake.restartedUnit)
	}
}

// devServerLifecycle records the unit ops in order and reports the unit gone
// once it has been stopped, the way launchd/systemd do.
type devServerLifecycle struct {
	ops     []string
	stopped bool
}

func (f *devServerLifecycle) Start(name string) error {
	f.ops = append(f.ops, "start "+name)
	return nil
}
func (f *devServerLifecycle) Restart(name string) error {
	f.ops = append(f.ops, "restart "+name)
	return nil
}
func (f *devServerLifecycle) Stop(name string) error {
	f.ops = append(f.ops, "stop "+name)
	f.stopped = true
	return nil
}
func (f *devServerLifecycle) UnitStatus(name string) (string, error) {
	if f.stopped {
		return "inactive", nil
	}
	return "active", nil
}
func (f *devServerLifecycle) AllUnitStates() map[string]string { return nil }

func addHostProxySite(t *testing.T, name string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	config.AddSite(config.Site{
		Name:        name,
		Domains:     []string{name + ".test"},
		Path:        t.TempDir(),
		HostPort:    5173,
		HostCommand: "npm run dev",
	})
}

// A dev server that drains its queues on SIGTERM keeps the port for a moment
// after the unit reports stopped, so the start has to wait for the port.
func TestRestartSite_HostProxyWaitsForPortRelease(t *testing.T) {
	addHostProxySite(t, "nuxtapp")

	fake := &devServerLifecycle{}
	podman.UnitLifecycle = fake
	defer func() { podman.UnitLifecycle = nil }()

	probes := 0
	prevProbe, prevPoll := devServerPortInUse, hostProxyStopPoll
	devServerPortInUse = func(port int) bool {
		probes++
		return probes < 3
	}
	hostProxyStopPoll = time.Millisecond
	defer func() { devServerPortInUse, hostProxyStopPoll = prevProbe, prevPoll }()

	if err := RestartSite("nuxtapp"); err != nil {
		t.Fatalf("RestartSite: %v", err)
	}
	want := []string{"stop lerd-app-nuxtapp", "start lerd-app-nuxtapp"}
	if len(fake.ops) != len(want) || fake.ops[0] != want[0] || fake.ops[1] != want[1] {
		t.Fatalf("ops = %v, want %v", fake.ops, want)
	}
	if probes < 3 {
		t.Errorf("port probed %d times, want the start held until the port was free", probes)
	}
}

// A port that never frees is usually something else holding it, and the dev
// server is the site's only runtime, so it is started regardless: the unit
// retries until the port comes back, where refusing would leave the site down
// with nothing to bring it up again.
func TestRestartSite_HostProxyStartsWhenPortNeverFrees(t *testing.T) {
	addHostProxySite(t, "stuckapp")

	fake := &devServerLifecycle{}
	podman.UnitLifecycle = fake
	defer func() { podman.UnitLifecycle = nil }()

	prevProbe, prevPoll, prevTimeout := devServerPortInUse, hostProxyStopPoll, hostProxyStopTimeout
	devServerPortInUse = func(port int) bool { return true }
	hostProxyStopPoll, hostProxyStopTimeout = time.Millisecond, 10*time.Millisecond
	defer func() {
		devServerPortInUse, hostProxyStopPoll, hostProxyStopTimeout = prevProbe, prevPoll, prevTimeout
	}()

	if err := RestartSite("stuckapp"); err != nil {
		t.Fatalf("RestartSite: %v", err)
	}
	want := []string{"stop lerd-app-stuckapp", "start lerd-app-stuckapp"}
	if len(fake.ops) != len(want) || fake.ops[0] != want[0] || fake.ops[1] != want[1] {
		t.Fatalf("ops = %v, want %v", fake.ops, want)
	}
}

// The gateway rebind runs unattended over every host-proxy site, so it waits
// far less than an explicit restart before giving up on the port.
func TestRebindWaitIsShorterThanRestartWait(t *testing.T) {
	if hostProxyRebindTimeout >= hostProxyStopTimeout {
		t.Fatalf("rebind wait %s, restart wait %s, want the rebind to be shorter",
			hostProxyRebindTimeout, hostProxyStopTimeout)
	}
}

func TestRestartDevServerHonoursItsWait(t *testing.T) {
	fake := &devServerLifecycle{}
	podman.UnitLifecycle = fake
	defer func() { podman.UnitLifecycle = nil }()

	prevProbe, prevPoll := devServerPortInUse, hostProxyStopPoll
	devServerPortInUse = func(port int) bool { return true }
	hostProxyStopPoll = time.Millisecond
	defer func() { devServerPortInUse, hostProxyStopPoll = prevProbe, prevPoll }()

	start := time.Now()
	if err := restartDevServer("lerd-app-rebound", 5173, 20*time.Millisecond); err != nil {
		t.Fatalf("restartDevServer: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waited %s on a 20ms budget", elapsed)
	}
}

func TestRestartSite_NotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	// Write an empty sites.yaml so FindSite returns not found.
	dir := filepath.Join(tmp, "lerd")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "sites.yaml"), []byte("sites: []\n"), 0644)

	err := RestartSite("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent site")
	}
}
