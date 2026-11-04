package lanip

import (
	"net"
	"testing"
)

// Whatever the host's network, Primary answers a usable IPv4 or nothing:
// never loopback, never IPv6.
func TestPrimaryIsEmptyOrANonLoopbackIPv4(t *testing.T) {
	got := Primary()
	if got == "" {
		return
	}
	ip := net.ParseIP(got)
	if ip == nil || ip.To4() == nil || ip.IsLoopback() {
		t.Fatalf("Primary() = %q, want a non-loopback IPv4", got)
	}
}

// A VPN that takes the default route (Cloudflare WARP's utun) must not become
// the address .test resolves to when the LAN is exposed.
func TestPrimaryIsNeverATunnelAddress(t *testing.T) {
	got := Primary()
	if got == "" {
		return
	}
	ifaces, _ := net.Interfaces()
	if IsTunnelAddr(ifaces, net.ParseIP(got)) {
		t.Fatalf("Primary() = %q, which belongs to a VPN tunnel", got)
	}
}

func TestIsTunnelInterface(t *testing.T) {
	for name, want := range map[string]bool{
		"utun8": true, "tun0": true, "wg0": true, "CloudflareWARP": true, "ppp0": true,
		"en0": false, "eth0": false, "wlan0": false, "bridge100": false,
	} {
		if got := IsTunnelInterface(name); got != want {
			t.Errorf("IsTunnelInterface(%q) = %v, want %v", name, got, want)
		}
	}
}
