// Package freeport provides a shared, dependency-free TCP host-port allocator:
// a dual-stack bindability probe and a predicate-injected free-port search. It
// is a leaf package (stdlib only) so both internal/cli (host-proxy dev servers)
// and internal/serviceops (the service port-ownership guard) can import it
// without an import cycle, and so the search logic stays unit-testable without
// binding real sockets.
package freeport

import (
	"net"
	"strconv"
)

// Bindable reports whether a TCP port can be bound across the addresses lerd's
// published quadlets and host-proxy dev servers publish on. A bind test is
// stricter and more accurate than a dial test for "can we publish here": it
// catches a port reserved on any stack, not just one with a live listener.
//
// Three probes must all succeed. The loopback specifics (127.0.0.1 and [::1])
// catch a server bound to a specific loopback address. The IPv4 wildcard
// (0.0.0.0) catches one bound to all interfaces — 0.0.0.0 or dual-stack [::] —
// which a specific-address bind slips past under SO_REUSEADDR on BSD/macOS, so
// probing only the loopback specifics reports a wildcard-bound host server
// (e.g. a MySQL on bind-address 0.0.0.0) as free and lets lerd collide with it.
// On macOS a running lerd container's gvproxy holds the dual-stack wildcard, so
// the wildcard probe reports its port in use; gvproxy releases it synchronously
// when the container stops, so a reinstall still rebinds without a spurious
// shift. A host with no IPv6 loopback at all is tolerated — the v6 check is
// skipped rather than treated as busy. Each listener is held open (deferred
// close) through the following binds so the set is tested atomically.
func Bindable(port int) bool {
	ln4w, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	defer ln4w.Close()
	ln4, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	defer ln4.Close()
	ln6, err := net.Listen("tcp", net.JoinHostPort("::1", strconv.Itoa(port)))
	if err != nil {
		// Distinguish "port already taken on ::1" from "this host has no IPv6 loopback".
		probe, perr := net.Listen("tcp", "[::1]:0")
		if perr != nil {
			return true // no IPv6 loopback here; the v4 binds are sufficient
		}
		_ = probe.Close()
		return false // IPv6 works but this port is taken on ::1
	}
	_ = ln6.Close()
	return true
}

// FirstFree returns the first port at or above start for which taken reports
// false. The predicate is injected so the search is unit-testable without
// binding real sockets; callers compose it from Bindable plus any reserved-port
// set of their own. start is clamped to >= 1. Returns 0 when nothing in
// [start, 65535] is free, so callers can decide their own fallback.
func FirstFree(start int, taken func(int) bool) int {
	if start < 1 {
		start = 1
	}
	for p := start; p <= 65535; p++ {
		if !taken(p) {
			return p
		}
	}
	return 0
}
