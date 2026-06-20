package serviceops

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAttributeTCP3306Owner(t *testing.T) {
	cases := []struct {
		listening, socketPresent, lerdRunning bool
		want                                  string
	}{
		{false, false, false, "none"},
		{false, true, true, "none"}, // not listening dominates
		{true, false, true, "lerd"},
		{true, true, true, "lerd"}, // a running lerd container wins attribution
		{true, true, false, "host"},
		{true, false, false, "unknown"},
	}
	for _, c := range cases {
		if got := attributeTCP3306Owner(c.listening, c.socketPresent, c.lerdRunning); got != c.want {
			t.Errorf("attributeTCP3306Owner(listening=%v,socket=%v,lerd=%v) = %q, want %q",
				c.listening, c.socketPresent, c.lerdRunning, got, c.want)
		}
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
