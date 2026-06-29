package serviceops

import (
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

	got := maybeShiftPublishedPort(busy, false)
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
	if got := maybeShiftPublishedPort(busy, true); got != 0 {
		t.Errorf("maybeShiftPublishedPort(busy, active=true) = %d, want 0 (never move a live service)", got)
	}

	// A non-positive primary (no host port published) is never shifted.
	if got := maybeShiftPublishedPort(0, false); got != 0 {
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
