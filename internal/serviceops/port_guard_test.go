package serviceops

import (
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The dual-stack bind probe and first-free search the guard builds on now live
// in internal/freeport (TestBindable_falseForBoundPort, TestFirstFree*); this
// file keeps the serviceops-specific pieces: the reserved-port set, fail-closed
// persistence, and the generic shift decision.

func TestLerdReservedPorts_includesPresetPort(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	// A stopped service pinned to its preset default port, with NO PublishedPort
	// override. Nothing is listening, so freeport.Bindable() would call it free — only
	// the reserved set keeps the auto-picker off it and prevents a boot-time collision.
	cfg.Services["mariadb-11"] = config.ServiceConfig{Enabled: true, Port: 13399}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	reserved := lerdReservedPorts()
	if !reserved[13399] {
		t.Errorf("lerdReservedPorts must reserve a service's preset default port 13399; got %v", reserved)
	}
}

// TestLerdReservedPorts_includesInstalledCustomService pins finding #6: an
// installed custom service's ports live in its own YAML, never in cfg.Services.
// The guard previously read only cfg.Services and so was blind to them, free to
// shift a built-in onto a stopped custom service's port and collide at boot. With
// the unified config.ReservedHostPorts the guard now sees them.
func TestLerdReservedPorts_includesInstalledCustomService(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	svc := &config.CustomService{Name: "my-thing", Image: "example/my-thing:1", Ports: []string{"34567:80"}}
	if err := config.SaveCustomService(svc); err != nil {
		t.Fatalf("SaveCustomService: %v", err)
	}

	if !lerdReservedPorts()[34567] {
		t.Errorf("the guard must reserve an installed custom service's host port 34567; got %v", lerdReservedPorts())
	}
}

func TestPersistPublishedPort_persists(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	if err := persistPublishedPort("postgres", 5433); err != nil {
		t.Fatalf("persistPublishedPort: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if got := cfg.Services["postgres"].PublishedPort; got != 5433 {
		t.Errorf("PublishedPort = %d, want 5433 (persisted)", got)
	}
}

func TestPersistPublishedPort_surfacesSaveFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only config dir is not enforced for root")
	}
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	t.Setenv("XDG_CONFIG_HOME", ro)
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// SaveGlobal can't write under a read-only config dir. The guard must surface
	// that failure (fail closed) rather than silently leave the port on the default
	// and write a colliding quadlet.
	if err := persistPublishedPort("postgres", 5433); err == nil {
		t.Error("persistPublishedPort must return an error when the config can't be saved")
	}
}

// freeLoopbackPort returns a port that was bindable a moment ago (bound to :0
// then released), for tests that need a port nothing is currently listening on.
func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a loopback port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// Issue #704: an installed sibling that already holds the shared canonical port
// makes the port "claimed"; a phantom default preset that is only seeded in
// config but never installed does not.
func TestPortClaimedByOtherInstalled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	sib := &config.CustomService{Name: "sib", Image: "example/x:1", Ports: []string{"33061:3306"}}
	if err := config.SaveCustomService(sib); err != nil {
		t.Fatalf("SaveCustomService: %v", err)
	}
	if !portClaimedByOtherInstalled("newsib", 33061) {
		t.Error("an installed sibling holding 33061 must count as a claim")
	}
	if portClaimedByOtherInstalled("sib", 33061) {
		t.Error("a service must not count as claiming its own port")
	}
	if portClaimedByOtherInstalled("newsib", 33062) {
		t.Error("no service holds 33062; must not be claimed")
	}

	// A default preset seeded in config but with no installed quadlet is a phantom
	// and must NOT claim its port, so a single-database install keeps the canonical.
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.Services["mysql"] = config.ServiceConfig{Enabled: true, Port: 33070}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	if portClaimedByOtherInstalled("mariadb", 33070) {
		t.Error("a seeded-but-uninstalled default preset must not claim its port (#704)")
	}
}

// Issue #704: the guard keeps a bindable canonical port when only a phantom
// (uninstalled) preset holds it, and shifts when an installed sibling does.
func TestGenericGuard_704CanonicalSharing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	canonical := freeLoopbackPort(t)

	// Only a phantom mysql (default preset, not installed) is seeded on the port.
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.Services["mysql"] = config.ServiceConfig{Enabled: true, Port: canonical}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	if got := maybeShiftPublishedPort("mariadb", canonical, false); got != 0 {
		t.Errorf("guard shifted off %d though only a phantom preset holds it = %d, want 0 (keep canonical)", canonical, got)
	}

	// Now an installed sibling genuinely holds the port: the guard must shift.
	sib := &config.CustomService{Name: "sib", Image: "example/x:1", Ports: []string{fmt.Sprintf("%d:3306", canonical)}}
	if err := config.SaveCustomService(sib); err != nil {
		t.Fatalf("SaveCustomService: %v", err)
	}
	if got := maybeShiftPublishedPort("mariadb", canonical, false); got <= canonical {
		t.Errorf("guard = %d, want a free port > %d once an installed sibling holds the canonical", got, canonical)
	}
}

// TestGenericGuard_shiftsWhenPrimaryBusy: when a service's primary host port is
// in use and the service itself is NOT up, the guard picks a later free port.
func TestGenericGuard_shiftsWhenPrimaryBusy(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a loopback port: %v", err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	got := maybeShiftPublishedPort("mysql-9-7", busy, false)
	if got <= busy {
		t.Errorf("maybeShiftPublishedPort(%d, active=false) = %d, want a free port > %d", busy, got, busy)
	}
}

// TestGenericGuard_sticksOncePersisted: the guard never moves a service whose
// own unit is up (its own listener isn't a foreign owner), and never moves one
// whose primary port is free. Combined with the published_port>0 gate in the
// caller, this is what makes an auto-shifted port stick rather than reshuffle.
func TestGenericGuard_sticksOncePersisted(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a loopback port: %v", err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	// Service is up: its own listener holds the port — do NOT shift it.
	if got := maybeShiftPublishedPort("mysql", busy, true); got != 0 {
		t.Errorf("maybeShiftPublishedPort(busy, active=true) = %d, want 0 (never move a live service)", got)
	}

	// A non-positive primary (no host port published) is never shifted.
	if got := maybeShiftPublishedPort("mysql", 0, false); got != 0 {
		t.Errorf("maybeShiftPublishedPort(0, active=false) = %d, want 0", got)
	}

	// The published_port==0 gate is what the caller consults; once a port is
	// recorded, ServicePublishedPort is non-zero and the probe is skipped entirely.
	if err := persistPublishedPort("redis", 6380); err != nil {
		t.Fatalf("persistPublishedPort: %v", err)
	}
	if config.ServicePublishedPort("redis") == 0 {
		t.Error("after a shift is persisted, ServicePublishedPort must be non-zero so the guard skips the probe")
	}
}
