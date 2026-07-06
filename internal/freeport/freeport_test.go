package freeport

import (
	"net"
	"testing"
)

// TestBindable_falseForBoundPort: a port with a live listener isn't bindable.
// (Relocated from serviceops port_guard_test.go as portBindable moved here.)
func TestBindable_falseForBoundPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if Bindable(port) {
		t.Errorf("Bindable(%d) = true for a bound port, want false", port)
	}
}

// TestBindable_falseForAllInterfacesBoundPort: a server listening on all
// interfaces must read as not bindable. Go binds every wildcard string
// ("0.0.0.0:0", ":0", "[::]:0") as a dual-stack [::] socket — the shape gvproxy
// uses to publish a container's port on macOS and that a DB configured to listen
// on all interfaces (bind-address *, listen_addresses '*') takes. A
// specific-loopback probe alone slips past it under SO_REUSEADDR on macOS and
// lets lerd publish a container on the same port; the wildcard probe catches it.
// Both wildcard spellings are exercised so the coverage is explicit.
func TestBindable_falseForAllInterfacesBoundPort(t *testing.T) {
	for _, spec := range []string{"0.0.0.0:0", "[::]:0"} {
		ln, err := net.Listen("tcp", spec)
		if err != nil {
			t.Skipf("cannot listen on %s here: %v", spec, err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		if Bindable(port) {
			t.Errorf("Bindable(%d) = true for a %s listener, want false", port, spec)
		}
		ln.Close()
	}
}

// TestFirstFree exercises the injected-predicate search: skipping taken ports,
// returning start when nothing is taken, clamping start < 1, and returning 0
// when everything is taken.
func TestFirstFree(t *testing.T) {
	taken := map[int]bool{40000: true, 40001: true}
	if got := FirstFree(40000, func(p int) bool { return taken[p] }); got != 40002 {
		t.Errorf("FirstFree skipping taken = %d, want 40002", got)
	}
	if got := FirstFree(40000, func(int) bool { return false }); got != 40000 {
		t.Errorf("FirstFree with none taken = %d, want 40000 (start)", got)
	}
	if got := FirstFree(0, func(int) bool { return false }); got != 1 {
		t.Errorf("FirstFree(0) = %d, want 1 (clamped)", got)
	}
	if got := FirstFree(65534, func(int) bool { return true }); got != 0 {
		t.Errorf("FirstFree all-taken = %d, want 0", got)
	}
}

// TestFirstFree_skipsBusyAndReserved composes Bindable with a reserved set, the
// way the service guard and the host-proxy allocator do, against a real bound
// port. (Relocated/adapted from serviceops TestFirstFreeHostPort_skipsBusyAndReserved.)
func TestFirstFree_skipsBusyAndReserved(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port
	reserved := map[int]bool{busy + 1: true, busy + 2: true}
	got := FirstFree(busy, func(p int) bool { return reserved[p] || !Bindable(p) })
	if got <= busy || reserved[got] {
		t.Errorf("FirstFree = %d, want a free port > %d skipping reserved %v", got, busy, reserved)
	}
}
