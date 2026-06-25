package serviceops

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestPortBindable_falseForBoundPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a loopback port: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if portBindable(port) {
		t.Errorf("port %d has a live listener but portBindable reported it free", port)
	}
}

func TestFirstFreeHostPort_skipsBusyAndReserved(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a loopback port: %v", err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	// Reserve the two ports just above the busy one; firstFreeHostPort must skip the
	// busy port AND the reserved ones and return a later, free port.
	reserved := map[int]bool{busy + 1: true, busy + 2: true}
	got := firstFreeHostPort(busy, reserved)
	if got == busy || reserved[got] {
		t.Errorf("firstFreeHostPort(%d) = %d; must skip the busy port and reserved %v", busy, got, reserved)
	}
	if got <= busy {
		t.Errorf("firstFreeHostPort(%d) = %d, want a port > start", busy, got)
	}
}

func TestLerdReservedPorts_includesPresetPort(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	// A stopped service pinned to its preset default port, with NO PublishedPort
	// override. Nothing is listening, so portBindable() would call it free — only the
	// reserved set keeps the auto-picker off it and prevents a boot-time collision.
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

func TestHostServerInstalled(t *testing.T) {
	defer config.SetHostDBGOOSForTest("linux")() // socket/marker detection is the Linux path
	root := t.TempDir()
	sockDir := filepath.Join(root, "run", "postgresql")
	marker := filepath.Join(root, "etc", "postgresql")
	if err := os.MkdirAll(sockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pg := config.HostBackendSpec{SocketIsDir: true, DefaultPort: 5432, LinuxSocket: sockDir, LinuxInstallMarkers: []string{marker}}

	// (a) The shared socket DIRECTORY alone — no socket file, no install marker — must NOT
	// count as a host server (the server-less postgresql-common / pgbouncer false positive).
	if hostServerInstalled(pg) {
		t.Error("bare socket dir (no socket file, no install marker) must NOT count as a host server")
	}
	// (a2) An EMPTY marker parent (e.g. /etc/postgresql shipped by postgresql-common with no
	// cluster) must NOT count — only a populated cluster dir does.
	if err := os.MkdirAll(marker, 0o755); err != nil {
		t.Fatal(err)
	}
	if hostServerInstalled(pg) {
		t.Error("empty install-marker dir (postgresql-common, no cluster) must NOT count as installed")
	}
	// (b) A POPULATED server marker (a real cluster dir lives inside) → installed, even with
	// nothing running (liveness-independent — the whole point of the boot-collision guard).
	if err := os.MkdirAll(filepath.Join(marker, "18", "main"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !hostServerInstalled(pg) {
		t.Error("populated install-marker dir should count as installed even with no running server")
	}

	// MySQL: the socket FILE is the running signal; the populated data dir is the install marker.
	mysqlSock := filepath.Join(root, "run", "mysqld", "mysqld.sock")
	mysqlData := filepath.Join(root, "var", "lib", "mysql")
	my := config.HostBackendSpec{SocketIsDir: false, DefaultPort: 3306, LinuxSocket: mysqlSock, LinuxInstallMarkers: []string{mysqlData}}
	if hostServerInstalled(my) {
		t.Error("mysql with no socket file and no data dir must NOT count as installed")
	}
	if err := os.MkdirAll(filepath.Join(mysqlData, "mysql"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !hostServerInstalled(my) {
		t.Error("mysql populated data-dir install marker should count as installed")
	}
}

func TestHostOwnsDBPort_skipsNonDBService(t *testing.T) {
	// redis has no host backend, so the port-ownership guard never probes or fires for it.
	if _, hostOwns, ok := hostOwnsDBPort("redis"); ok || hostOwns {
		t.Errorf("redis must not be treated as a host-capable DB service (ok=%v hostOwns=%v)", ok, hostOwns)
	}
}

func TestHostOwnsDBPort_installedHostPostgres(t *testing.T) {
	// Integration check against THIS machine: if a host Postgres SERVER is installed (its
	// cluster config dir exists), the guard must classify it host-owned — independent of
	// whether the cluster is running (the fix for the boot-collision race). Skips where no
	// server is installed; a server-less postgresql-common box must NOT trip it.
	if entries, err := os.ReadDir("/etc/postgresql"); err != nil || len(entries) == 0 {
		t.Skip("no host Postgres server cluster on this machine")
	}
	defer config.SetHostDBGOOSForTest("linux")()
	if _, hostOwns, ok := hostOwnsDBPort("postgres"); !ok || !hostOwns {
		t.Errorf("installed host Postgres must classify host-owned (ok=%v hostOwns=%v)", ok, hostOwns)
	}
}
