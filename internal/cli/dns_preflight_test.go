package cli

import (
	"net"
	"runtime"
	"strings"
	"testing"
)

func TestPreflightForwarderPort_OwnUnitActiveSkipsCheck(t *testing.T) {
	prevStatus := forwarderUnitStatusFn
	prevFree := forwarderPortFreeFn
	prevHolder := forwarderPortHolderFn
	t.Cleanup(func() {
		forwarderUnitStatusFn = prevStatus
		forwarderPortFreeFn = prevFree
		forwarderPortHolderFn = prevHolder
	})

	forwarderUnitStatusFn = func() string { return "active" }
	forwarderPortFreeFn = func(string, int) bool {
		t.Error("forwarderPortFreeFn must not be called when our forwarder is active")
		return true
	}
	forwarderPortHolderFn = func(string, int) string {
		t.Error("forwarderPortHolderFn must not be called when our forwarder is active")
		return ""
	}

	if err := preflightForwarderPort("192.168.1.10"); err != nil {
		t.Errorf("preflight should pass when our forwarder is active, got %v", err)
	}
}

func TestPreflightForwarderPort_PortFreeReturnsNil(t *testing.T) {
	prevStatus := forwarderUnitStatusFn
	prevFree := forwarderPortFreeFn
	t.Cleanup(func() {
		forwarderUnitStatusFn = prevStatus
		forwarderPortFreeFn = prevFree
	})

	forwarderUnitStatusFn = func() string { return "inactive" }
	forwarderPortFreeFn = func(string, int) bool { return true }

	if err := preflightForwarderPort("192.168.1.10"); err != nil {
		t.Errorf("preflight should pass when port is free, got %v", err)
	}
}

func TestPreflightForwarderPort_PortInUseSurfacesHolder(t *testing.T) {
	prevStatus := forwarderUnitStatusFn
	prevFree := forwarderPortFreeFn
	prevHolder := forwarderPortHolderFn
	t.Cleanup(func() {
		forwarderUnitStatusFn = prevStatus
		forwarderPortFreeFn = prevFree
		forwarderPortHolderFn = prevHolder
	})

	forwarderUnitStatusFn = func() string { return "inactive" }
	forwarderPortFreeFn = func(string, int) bool { return false }
	forwarderPortHolderFn = func(host string, port int) string {
		return "  dnsmasq    1234 root  6u  IPv4  0x0  UDP " + host + ":5300"
	}

	err := preflightForwarderPort("192.168.1.10")
	if err == nil {
		t.Fatal("expected preflight to fail when port is in use")
	}
	msg := err.Error()
	for _, want := range []string{"192.168.1.10:5300", "already in use", "dnsmasq", "lerd lan expose"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q\nfull message: %s", want, msg)
		}
	}
}

func TestForwarderPortFree_DetectsBoundUDP(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("could not bind a UDP port for the test: %v", err)
	}
	defer conn.Close()
	port := conn.LocalAddr().(*net.UDPAddr).Port

	if forwarderPortFree("127.0.0.1", port) {
		t.Errorf("expected forwarderPortFree to report port %d as taken", port)
	}
}

func TestForwarderPortFree_DetectsBoundTCP(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("could not bind a TCP port for the test: %v", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	if forwarderPortFree("127.0.0.1", port) {
		t.Errorf("expected forwarderPortFree to report TCP-bound port %d as taken", port)
	}
}

func TestForwarderPortFree_FreePortReturnsTrue(t *testing.T) {
	// Get a free port by binding, reading the port, closing.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("could not pick a free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	if !forwarderPortFree("127.0.0.1", port) {
		t.Errorf("expected just-released port %d to read as free", port)
	}
}

func TestForwarderPortHolderLsof_FallbackHintTailoredByOS(t *testing.T) {
	// We can't reliably force lsof to fail / succeed in a unit test, but
	// we can verify the fallback string for a port that's almost certainly
	// not bound (so lsof returns empty and we hit the fallback branch).
	hint := forwarderPortHolderLsof("203.0.113.0", 65500)
	if runtime.GOOS == "linux" && !strings.Contains(hint, "ss -tulpn") {
		t.Errorf("Linux fallback should suggest ss, got %q", hint)
	}
	if runtime.GOOS == "darwin" && !strings.Contains(hint, "lsof") {
		t.Errorf("macOS fallback should suggest lsof, got %q", hint)
	}
}
