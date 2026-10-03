package dns

import (
	"net"
	"strconv"
	"testing"
	"time"
)

// WaitReady has to probe the port lerd-dns is configured on, not a fixed one:
// Windows serves DNS on 53, since its resolver rule cannot name a port.
func TestWaitReadyProbesTheConfiguredPort(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	old := dnsPort
	t.Cleanup(func() { dnsPort = old })
	dnsPort = port
	if err := WaitReady(2 * time.Second); err != nil {
		t.Fatalf("a listener on the configured port %d was not seen: %v", port, err)
	}

	_ = ln.Close()
	dnsPort = port
	if err := WaitReady(300 * time.Millisecond); err == nil {
		t.Error("with nothing listening on " + strconv.Itoa(port) + " WaitReady must time out")
	}
}
