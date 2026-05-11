package config

import (
	"runtime"
	"strings"
	"testing"
)

// TestDumpsListenNetwork pins the per-platform transport switch the
// dump bridge relies on. macOS can't reach a unix socket on the host
// from inside the podman-machine VM, so the receiver listens on TCP
// instead; Linux's bind-mounted unix socket is reachable from FPM
// directly.
func TestDumpsListenNetwork(t *testing.T) {
	got := DumpsListenNetwork()
	switch runtime.GOOS {
	case "darwin":
		if got != "tcp" {
			t.Errorf("darwin: got %q, want %q", got, "tcp")
		}
	default:
		if got != "unix" {
			t.Errorf("non-darwin: got %q, want %q", got, "unix")
		}
	}
}

func TestDumpsListenAddr(t *testing.T) {
	got := DumpsListenAddr()
	switch runtime.GOOS {
	case "darwin":
		want := "127.0.0.1:" + DumpsTCPPort
		if got != want {
			t.Errorf("darwin: got %q, want %q", got, want)
		}
	default:
		// Linux: addr is the unix socket path under RunDir.
		want := DumpsSocketPath()
		if got != want {
			t.Errorf("non-darwin: got %q, want %q", got, want)
		}
	}
}

// TestDumpsBridgeTarget pins the PHP-side connection string the
// FPM dump bridge reads from its conf.d ini. The scheme must include
// the protocol prefix (`tcp://` or `unix://`) so PHP's
// stream_socket_client routes to the right backend.
func TestDumpsBridgeTarget(t *testing.T) {
	got := DumpsBridgeTarget()
	switch runtime.GOOS {
	case "darwin":
		want := "tcp://host.containers.internal:" + DumpsTCPPort
		if got != want {
			t.Errorf("darwin: got %q, want %q", got, want)
		}
	default:
		want := "unix://" + DumpsSocketPath()
		if got != want {
			t.Errorf("non-darwin: got %q, want %q", got, want)
		}
	}
	if !strings.Contains(got, "://") {
		t.Errorf("bridge target should be a URL with a scheme; got %q", got)
	}
}

// TestDumpsTCPPort_consistency pins that the constant the listen path
// and the bridge target use is the same string. A mismatch here would
// silently send dumps to a port no one listens on.
func TestDumpsTCPPort_consistency(t *testing.T) {
	if !strings.Contains(DumpsListenAddr(), DumpsTCPPort) && runtime.GOOS == "darwin" {
		t.Errorf("listen addr %q must contain DumpsTCPPort %q on darwin", DumpsListenAddr(), DumpsTCPPort)
	}
	if !strings.Contains(DumpsBridgeTarget(), DumpsTCPPort) && runtime.GOOS == "darwin" {
		t.Errorf("bridge target %q must contain DumpsTCPPort %q on darwin", DumpsBridgeTarget(), DumpsTCPPort)
	}
}
