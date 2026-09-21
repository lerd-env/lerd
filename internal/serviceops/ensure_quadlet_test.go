package serviceops

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

func TestEnsureCustomServiceQuadlet_reloadsOnlyWhenContentChanges(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	count := 0
	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error {
		count++
		return nil
	}

	svc := &config.CustomService{
		Name:  "mongo-express",
		Image: "docker.io/library/mongo-express:latest",
		Ports: []string{"127.0.0.1:8082:8081"},
	}

	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("first EnsureCustomServiceQuadlet: %v", err)
	}
	if count != 1 {
		t.Errorf("first call should reload once, got %d", count)
	}

	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("second EnsureCustomServiceQuadlet: %v", err)
	}
	if count != 1 {
		t.Errorf("second call with unchanged content must not reload, got %d total", count)
	}

	svc.Image = "docker.io/library/mongo-express:1.0.2"
	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("third EnsureCustomServiceQuadlet: %v", err)
	}
	if count != 2 {
		t.Errorf("changed image should reload again, got %d total", count)
	}
}

// TestEnsureCustomServiceQuadlet_shiftsBusySecondaryPort: the ownership guard
// covers every published mapping, not just the primary. When a multi-port
// service's secondary host port is already taken while its primary is free, the
// guard shifts only the secondary to a free port and persists it under
// published_ports, keyed by the mapping's container port, so the unit still binds.
func TestEnsureCustomServiceQuadlet_shiftsBusySecondaryPort(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }

	// Occupy the secondary host port; leave the primary free.
	busyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a loopback port: %v", err)
	}
	defer busyLn.Close()
	busySecondary := busyLn.Addr().(*net.TCPAddr).Port

	freeLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a loopback port: %v", err)
	}
	freePrimary := freeLn.Addr().(*net.TCPAddr).Port
	freeLn.Close() // release so the primary mapping binds cleanly and isn't shifted

	svc := &config.CustomService{
		Name:  "twoport",
		Image: "docker.io/example/twoport:latest",
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:5000", freePrimary),
			fmt.Sprintf("127.0.0.1:%d:8025", busySecondary),
		},
	}
	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("EnsureCustomServiceQuadlet: %v", err)
	}

	if config.ServicePublishedPort("twoport") != 0 {
		t.Errorf("the free primary must not be shifted, got %d", config.ServicePublishedPort("twoport"))
	}
	moved := config.ServicePublishedPorts("twoport")[8025]
	if moved == 0 {
		t.Fatal("the busy secondary must be shifted and persisted under published_ports[8025]")
	}
	if moved == busySecondary {
		t.Errorf("the secondary must move off the busy port %d, got %d", busySecondary, moved)
	}
}

// TestEnsureCustomServiceQuadlet_materialisesHostsMountSource: the generated
// quadlet mounts the managed hosts file at /etc/hosts, and podman creates a
// directory at a missing Volume source, so the quadlet write must not be the
// first thing that touches that path. WriteFPMQuadlet already guarantees this
// through ensureFPMHostsFile; the custom-service path needs the same guarantee
// because `lerd service install` never goes near WriteContainerHosts.
func TestEnsureCustomServiceQuadlet_materialisesHostsMountSource(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }

	svc := &config.CustomService{
		Name:  "mongo-express",
		Image: "docker.io/library/mongo-express:latest",
		Ports: []string{"127.0.0.1:8082:8081"},
	}
	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("EnsureCustomServiceQuadlet: %v", err)
	}

	info, err := os.Stat(config.ContainerHostsFile())
	if err != nil {
		t.Fatalf("hosts mount source not materialised: %v", err)
	}
	if info.IsDir() {
		t.Error("hosts mount source is a directory, the service would inherit the host /etc/hosts")
	}
}

// TestEnsureCustomServiceQuadlet_healsStaleHostsDirectory: a previous broken
// start (or the macOS precreateBindMountDirs pass that runs off WriteQuadletDiff)
// leaves a directory at the mount source. It has to be healed into a regular
// file, otherwise every later WriteContainerHosts fails with "is a directory"
// and only warns.
func TestEnsureCustomServiceQuadlet_healsStaleHostsDirectory(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }

	hostsPath := config.ContainerHostsFile()
	if err := os.MkdirAll(hostsPath, 0755); err != nil {
		t.Fatal(err)
	}

	svc := &config.CustomService{
		Name:  "mongo-express",
		Image: "docker.io/library/mongo-express:latest",
		Ports: []string{"127.0.0.1:8082:8081"},
	}
	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("EnsureCustomServiceQuadlet: %v", err)
	}

	info, err := os.Stat(hostsPath)
	if err != nil {
		t.Fatalf("path missing after heal: %v", err)
	}
	if info.IsDir() {
		t.Error("path is still a directory, heal failed")
	}
}

// TestEnsureCustomServiceQuadlet_materialisesBrowserHostsForShareHosts: a
// share_hosts service mounts the browser variant instead, so that is the path
// needing the guarantee for those.
func TestEnsureCustomServiceQuadlet_materialisesBrowserHostsForShareHosts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }

	svc := &config.CustomService{
		Name:       "selenium",
		Image:      "docker.io/selenium/standalone-chromium:latest",
		Ports:      []string{"127.0.0.1:4444:4444"},
		ShareHosts: true,
	}
	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("EnsureCustomServiceQuadlet: %v", err)
	}

	info, err := os.Stat(config.BrowserHostsFile())
	if err != nil {
		t.Fatalf("browser hosts mount source not materialised: %v", err)
	}
	if info.IsDir() {
		t.Error("browser hosts mount source is a directory")
	}
}

// TestEnsureCustomServiceQuadlet_reshiftsRecordedPortAnotherServiceHolds: a
// recorded published port sticks, except when another installed service already
// publishes it. `service remove` keeps the removed service's config entry, so its
// port can be handed on and then reclaimed by a reinstall; publishing it anyway
// would put two units on one port at boot.
func TestEnsureCustomServiceQuadlet_reshiftsRecordedPortAnotherServiceHolds(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }
	origStatus := ensureUnitStatus
	t.Cleanup(func() { ensureUnitStatus = origStatus })
	ensureUnitStatus = func(string) (string, error) { return "inactive", nil }

	familyPort, handedOn, free := freeLoopbackPort(t), freeLoopbackPort(t), freeLoopbackPort(t)
	holder := &config.CustomService{
		Name:  "holder",
		Image: "example/x:1",
		Ports: []string{fmt.Sprintf("%d:3306", handedOn)},
	}
	if err := config.SaveCustomService(holder); err != nil {
		t.Fatalf("SaveCustomService: %v", err)
	}
	if err := persistPublishedPort("comeback", handedOn); err != nil {
		t.Fatalf("persistPublishedPort: %v", err)
	}

	svc := &config.CustomService{
		Name:  "comeback",
		Image: "example/x:1",
		Ports: []string{fmt.Sprintf("%d:3306", familyPort)},
	}
	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("EnsureCustomServiceQuadlet: %v", err)
	}
	if got := config.ServicePublishedPort("comeback"); got == handedOn {
		t.Errorf("kept published port %d though holder already publishes it", got)
	}
	if got := podman.PrimaryHostPort(svc.Ports); got == handedOn {
		t.Errorf("rendered mapping publishes %d, the port holder already binds", got)
	}

	// A recorded port no other service holds is left exactly where it is.
	if err := persistPublishedPort("comeback", free); err != nil {
		t.Fatalf("persistPublishedPort: %v", err)
	}
	svc.Ports = []string{fmt.Sprintf("%d:3306", familyPort)}
	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("EnsureCustomServiceQuadlet: %v", err)
	}
	if got := config.ServicePublishedPort("comeback"); got != free {
		t.Errorf("published port = %d, want the recorded %d to stick", got, free)
	}
}

// TestEnsureCustomServiceQuadlet_portShiftNoticeAvoidsStdout: when the port guard
// shifts a service off a busy port it must not write its notice to os.Stdout.
// EnsureCustomServiceQuadlet is called in-process by the MCP stdio server, which
// reserves os.Stdout for the JSON-RPC stream — any stray write there corrupts the
// protocol frame and breaks the client session.
func TestEnsureCustomServiceQuadlet_portShiftNoticeAvoidsStdout(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }

	// The guard leaves the port alone while the service's own unit is up, so pin
	// the unit down: otherwise a developer running lerd-mongo-express fails here.
	origStatus := ensureUnitStatus
	t.Cleanup(func() { ensureUnitStatus = origStatus })
	ensureUnitStatus = func(string) (string, error) { return "inactive", nil }

	// Occupy the service's primary host port so the guard is forced to shift it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a loopback port: %v", err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w

	svc := &config.CustomService{
		Name:  "mongo-express",
		Image: "docker.io/library/mongo-express:latest",
		Ports: []string{fmt.Sprintf("127.0.0.1:%d:8081", busy)},
	}
	ensErr := EnsureCustomServiceQuadlet(svc)
	_ = w.Close()
	os.Stdout = saved

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	if ensErr != nil {
		t.Fatalf("EnsureCustomServiceQuadlet: %v", ensErr)
	}
	if config.ServicePublishedPort("mongo-express") == 0 {
		t.Fatal("expected the guard to shift the busy port and persist a published port")
	}
	if buf.Len() != 0 {
		t.Errorf("port-shift notice leaked to os.Stdout (would corrupt MCP JSON-RPC): %q", buf.String())
	}
}

// TestEnsureCustomServiceQuadlet_reshiftsRecordedPortTakenByHost pins #1917: a
// recorded published port is only good while nothing else holds it. Something
// bound it while the service was down, so starting on it fails at the bind; the
// guard has to move the service the same way it moves one off a taken default.
func TestEnsureCustomServiceQuadlet_reshiftsRecordedPortTakenByHost(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }
	origStatus := ensureUnitStatus
	t.Cleanup(func() { ensureUnitStatus = origStatus })
	ensureUnitStatus = func(string) (string, error) { return "inactive", nil }

	// A squatter holds the port the service is recorded on; its own default is free.
	squatter, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a loopback port: %v", err)
	}
	defer squatter.Close()
	taken := squatter.Addr().(*net.TCPAddr).Port
	def := freeLoopbackPort(t)

	if err := persistPublishedPort("objects", taken); err != nil {
		t.Fatalf("persistPublishedPort: %v", err)
	}
	svc := &config.CustomService{
		Name:  "objects",
		Image: "example/objects:1",
		Ports: []string{fmt.Sprintf("127.0.0.1:%d:9000", def)},
	}
	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("EnsureCustomServiceQuadlet: %v", err)
	}

	moved := config.ServicePublishedPort("objects")
	if moved == taken {
		t.Fatalf("published port stayed on %d, the port a host process already binds", taken)
	}
	if moved == 0 {
		t.Fatal("the shift must be persisted, so the quadlet never publishes a port config does not record")
	}
	if got := podman.PrimaryHostPort(svc.Ports); got != moved {
		t.Errorf("rendered mapping publishes %d, want the shifted %d", got, moved)
	}
}

// TestEnsureCustomServiceQuadlet_reshiftsRecordedSecondaryPortTakenByHost: the
// same for a secondary mapping (an object store's console, a mail catcher's web
// UI), whose override is recorded per container port.
func TestEnsureCustomServiceQuadlet_reshiftsRecordedSecondaryPortTakenByHost(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }
	origStatus := ensureUnitStatus
	t.Cleanup(func() { ensureUnitStatus = origStatus })
	ensureUnitStatus = func(string) (string, error) { return "inactive", nil }

	squatter, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a loopback port: %v", err)
	}
	defer squatter.Close()
	taken := squatter.Addr().(*net.TCPAddr).Port
	primary, secondary := freeLoopbackPort(t), freeLoopbackPort(t)

	if err := persistPublishedPortFor("objects", 9001, taken); err != nil {
		t.Fatalf("persistPublishedPortFor: %v", err)
	}
	svc := &config.CustomService{
		Name:  "objects",
		Image: "example/objects:1",
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:9000", primary),
			fmt.Sprintf("127.0.0.1:%d:9001", secondary),
		},
	}
	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("EnsureCustomServiceQuadlet: %v", err)
	}

	moved := config.ServicePublishedPorts("objects")[9001]
	if moved == taken || moved == 0 {
		t.Fatalf("published_ports[9001] = %d, want a free port off the taken %d", moved, taken)
	}
	rendered := 0
	for _, spec := range svc.Ports {
		if podman.ContainerPort(spec) == 9001 {
			rendered = podman.PrimaryHostPort([]string{spec})
		}
	}
	if rendered != moved {
		t.Errorf("rendered console mapping publishes %d, want the shifted %d", rendered, moved)
	}
}

// TestEnsureCustomServiceQuadlet_shiftSyncsDashboardVhost: a service that moves
// may be one lerd's vhost predates, and the file answers only the paths it
// names, so the dashboard mount has to be re-rendered when the guard shifts.
func TestEnsureCustomServiceQuadlet_shiftSyncsDashboardVhost(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }
	origStatus := ensureUnitStatus
	t.Cleanup(func() { ensureUnitStatus = origStatus })
	ensureUnitStatus = func(string) (string, error) { return "inactive", nil }

	synced := 0
	origSync := syncLerdVhostFn
	t.Cleanup(func() { syncLerdVhostFn = origSync })
	syncLerdVhostFn = func() (bool, error) {
		synced++
		return false, nil
	}

	free := freeLoopbackPort(t)
	svc := &config.CustomService{
		Name:  "objects",
		Image: "example/objects:1",
		Ports: []string{fmt.Sprintf("127.0.0.1:%d:9000", free)},
	}
	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("EnsureCustomServiceQuadlet: %v", err)
	}
	if synced != 0 {
		t.Fatalf("a quadlet write that moves nothing must not touch the vhost, synced %d times", synced)
	}

	squatter, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a loopback port: %v", err)
	}
	defer squatter.Close()
	taken := squatter.Addr().(*net.TCPAddr).Port
	if err := persistPublishedPort("objects", taken); err != nil {
		t.Fatalf("persistPublishedPort: %v", err)
	}
	svc.Ports = []string{fmt.Sprintf("127.0.0.1:%d:9000", free)}
	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("EnsureCustomServiceQuadlet: %v", err)
	}
	if synced != 1 {
		t.Errorf("a shift must re-render lerd's vhost once, synced %d times", synced)
	}
}

// TestEnsureCustomServiceQuadlet_shiftsWhileUnitRestartsOnABindFailure: systemd
// restarts a unit that cannot bind, and reports it as "activating" for as long
// as it keeps trying, so unit state alone names a service that cannot start as
// the owner of the port it cannot bind. What settles it is whether its own
// container is running; when it is not, the port belongs to someone else.
func TestEnsureCustomServiceQuadlet_shiftsWhileUnitRestartsOnABindFailure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }
	origStatus := ensureUnitStatus
	t.Cleanup(func() { ensureUnitStatus = origStatus })
	ensureUnitStatus = func(string) (string, error) { return "activating", nil }
	origRunning := ensureContainerRunning
	t.Cleanup(func() { ensureContainerRunning = origRunning })
	ensureContainerRunning = func(string) bool { return false }

	squatter, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a loopback port: %v", err)
	}
	defer squatter.Close()
	taken := squatter.Addr().(*net.TCPAddr).Port

	svc := &config.CustomService{
		Name:  "objects",
		Image: "example/objects:1",
		Ports: []string{fmt.Sprintf("127.0.0.1:%d:9000", taken)},
	}
	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		t.Fatalf("EnsureCustomServiceQuadlet: %v", err)
	}
	if got := config.ServicePublishedPort("objects"); got == 0 || got == taken {
		t.Fatalf("published port = %d, want a shift off the taken %d while the unit restarts", got, taken)
	}

	// A service whose container is genuinely up owns the port: never move it.
	ensureContainerRunning = func(string) bool { return true }
	ensureUnitStatus = func(string) (string, error) { return "active", nil }
	if err := persistPublishedPort("held", taken); err != nil {
		t.Fatalf("persistPublishedPort: %v", err)
	}
	up := &config.CustomService{
		Name:  "held",
		Image: "example/held:1",
		Ports: []string{fmt.Sprintf("127.0.0.1:%d:9000", taken)},
	}
	if err := EnsureCustomServiceQuadlet(up); err != nil {
		t.Fatalf("EnsureCustomServiceQuadlet: %v", err)
	}
	if got := config.ServicePublishedPort("held"); got != taken {
		t.Errorf("published port = %d, want the running service left on its own %d", got, taken)
	}
}
