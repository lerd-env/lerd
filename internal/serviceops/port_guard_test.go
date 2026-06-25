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
