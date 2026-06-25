package serviceops

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAttributePortOwner(t *testing.T) {
	cases := []struct {
		listening, socketPresent, lerdRunning, lerdOnDefault bool
		want                                                 string
	}{
		// lerdOnDefault=true: identical to the previous three-signal behaviour.
		{false, false, false, true, "none"},
		{false, true, true, true, "none"}, // not listening dominates
		{true, false, true, true, "lerd"},
		{true, true, true, true, "lerd"}, // a running lerd on its default port wins attribution
		{true, true, false, true, "host"},
		{true, false, false, true, "unknown"},
		// lerdOnDefault=false: lerd moved off the default port, so a listener there is the host.
		{false, true, true, false, "none"},     // nothing on the default port
		{true, true, true, false, "host"},      // lerd on e.g. 3307, host socket on 3306
		{true, false, true, false, "host"},     // lerd elsewhere, default port listening ⇒ host
		{true, true, false, false, "host"},     // host socket present, lerd down
		{true, false, false, false, "unknown"}, // default port listening, no socket, lerd down
	}
	for _, c := range cases {
		if got := attributePortOwner(c.listening, c.socketPresent, c.lerdRunning, c.lerdOnDefault); got != c.want {
			t.Errorf("attributePortOwner(listening=%v,socket=%v,lerd=%v,lerdOnDefault=%v) = %q, want %q",
				c.listening, c.socketPresent, c.lerdRunning, c.lerdOnDefault, got, c.want)
		}
	}
}

// TestProbeHostDB_FamilyWiring checks the deterministic, environment-independent
// fields of the probe: the service name echoes back and the probed port is the
// engine's canonical port (3306 for MySQL, 5432 for Postgres), proving the probe
// is driven by the per-family spec rather than a hardcoded 3306.
func TestProbeHostDB_FamilyWiring(t *testing.T) {
	if got := ProbeHostDB("mysql", ""); got.ServiceName != "mysql" || got.Port != 3306 {
		t.Errorf("ProbeHostDB(mysql): ServiceName=%q Port=%d, want mysql/3306", got.ServiceName, got.Port)
	}
	if got := ProbeHostDB("postgres", ""); got.ServiceName != "postgres" || got.Port != 5432 {
		t.Errorf("ProbeHostDB(postgres): ServiceName=%q Port=%d, want postgres/5432", got.ServiceName, got.Port)
	}
	// An empty service name defaults to mysql (back-compat with ProbeHostMySQL).
	if got := ProbeHostDB("", ""); got.ServiceName != "mysql" || got.Port != 3306 {
		t.Errorf("ProbeHostDB(\"\"): ServiceName=%q Port=%d, want mysql/3306", got.ServiceName, got.Port)
	}
	// A family alternate (mariadb-11) resolves to its family's spec (mariadb → 3306)
	// and echoes the full service name — the container queried is lerd-mariadb-11,
	// not the canonical lerd-mariadb.
	if got := ProbeHostDB("mariadb-11", ""); got.ServiceName != "mariadb-11" || got.Port != 3306 {
		t.Errorf("ProbeHostDB(mariadb-11): ServiceName=%q Port=%d, want mariadb-11/3306", got.ServiceName, got.Port)
	}
	// A non-host-capable service yields an empty (zero-port) status.
	if got := ProbeHostDB("redis", ""); got.Port != 0 {
		t.Errorf("ProbeHostDB(redis): Port=%d, want 0 (no host backend)", got.Port)
	}
}

func TestSocketLive(t *testing.T) {
	dir := t.TempDir()

	// 1. Missing path.
	if p, l := socketLive(filepath.Join(dir, "nope.sock"), 200*time.Millisecond); p || l {
		t.Errorf("missing socket: present=%v live=%v, want false/false", p, l)
	}

	// 2. A regular file is not a socket.
	reg := filepath.Join(dir, "regular")
	if err := os.WriteFile(reg, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p, l := socketLive(reg, 200*time.Millisecond); p || l {
		t.Errorf("regular file: present=%v live=%v, want false/false", p, l)
	}

	// 3. A live unix socket with an accepting server.
	livePath := filepath.Join(dir, "live.sock")
	ln, err := net.Listen("unix", livePath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			c.Close()
		}
	}()
	if p, l := socketLive(livePath, 500*time.Millisecond); !p || !l {
		t.Errorf("live socket: present=%v live=%v, want true/true", p, l)
	}

	// 4. A stale socket file (inode present, nothing accepting).
	stalePath := filepath.Join(dir, "stale.sock")
	sln, err := net.Listen("unix", stalePath)
	if err != nil {
		t.Fatal(err)
	}
	sln.(*net.UnixListener).SetUnlinkOnClose(false) // leave the socket file behind
	sln.Close()
	if p, l := socketLive(stalePath, 300*time.Millisecond); !p || l {
		t.Errorf("stale socket: present=%v live=%v, want true/false", p, l)
	}
}

func TestTCPListening(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			c.Close()
		}
	}()
	addr := ln.Addr().String()
	if !tcpListening(addr, 500*time.Millisecond) {
		t.Errorf("tcpListening(%s) = false, want true", addr)
	}
	ln.Close()
	if tcpListening(addr, 200*time.Millisecond) {
		t.Error("tcpListening on a closed port = true, want false")
	}
}
