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

const (
	up  = net.FlagUp
	p2p = net.FlagUp | net.FlagPointToPoint
)

func ips(s ...string) []net.IP {
	out := make([]net.IP, len(s))
	for i, v := range s {
		out[i] = net.ParseIP(v)
	}
	return out
}

// The macOS host under Cloudflare WARP: the default route goes through utun8.
var warpHost = []nic{
	{name: "lo0", flags: up | net.FlagLoopback, ips: ips("127.0.0.1", "::1")},
	{name: "bridge100", flags: up, ips: ips("192.168.64.1")},
	{name: "en0", flags: up, ips: ips("192.168.1.155", "fe80::1", "2001:db8:42::7")},
	{name: "utun8", flags: p2p, ips: ips("172.16.0.2", "2606:4700:110::1")},
}

func TestPickV4(t *testing.T) {
	cases := []struct {
		name   string
		dialed string
		nics   []nic
		want   string
	}{
		{"no VPN keeps the dialed address", "192.168.1.155", warpHost, "192.168.1.155"},
		{"WARP tunnel falls back to the physical LAN", "172.16.0.2", warpHost, "192.168.1.155"},
		{"failed dial falls back to the physical LAN", "", warpHost, "192.168.1.155"},
		{"custom-named tunnel caught by the point-to-point flag", "10.64.0.9", []nic{
			{name: "acmevpn0", flags: p2p, ips: ips("10.64.0.9")},
			{name: "eth0", flags: up, ips: ips("10.0.0.20")},
		}, "10.0.0.20"},
		{"branded tunnel names", "10.2.0.2", []nic{
			{name: "proton0", flags: up, ips: ips("10.2.0.2")},
			{name: "cscotun0", flags: up, ips: ips("10.3.0.2")},
			{name: "wlan0", flags: up, ips: ips("192.168.0.4")},
		}, "192.168.0.4"},
		{"only a public address left", "", []nic{
			{name: "eth0", flags: up, ips: ips("203.0.113.5")},
		}, "203.0.113.5"},
		{"down interface skipped", "", []nic{
			{name: "en1", flags: 0, ips: ips("192.168.9.9")},
			{name: "utun0", flags: p2p, ips: ips("172.16.0.2")},
		}, ""},
		{"lerd's dummy link is never the LAN address", "", []nic{
			{name: "enp1s0", flags: 0, ips: ips("192.168.122.92")},
			{name: "lerd0", flags: up, ips: ips("192.0.2.1")},
		}, ""},
		{"a public address still wins over the dummy link", "", []nic{
			{name: "lerd0", flags: up, ips: ips("192.0.2.1")},
			{name: "eth0", flags: up, ips: ips("203.0.113.5")},
		}, "203.0.113.5"},
	}
	for _, c := range cases {
		if got := pickV4(net.ParseIP(c.dialed), c.nics); got != c.want {
			t.Errorf("%s: pickV4 = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPickV6(t *testing.T) {
	cases := []struct {
		name   string
		dialed string
		nics   []nic
		want   string
	}{
		{"no VPN keeps the dialed address", "2001:db8:42::7", warpHost, "2001:db8:42::7"},
		{"WARP tunnel falls back to the physical global v6", "2606:4700:110::1", warpHost, "2001:db8:42::7"},
		{"no global v6 off the tunnel means no AAAA", "2606:4700:110::1", []nic{
			{name: "en0", flags: up, ips: ips("192.168.1.155", "fe80::1", "fd00::5")},
			{name: "utun8", flags: p2p, ips: ips("2606:4700:110::1")},
		}, ""},
	}
	for _, c := range cases {
		if got := pickV6(net.ParseIP(c.dialed), c.nics); got != c.want {
			t.Errorf("%s: pickV6 = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestIsTunnelName(t *testing.T) {
	for name, want := range map[string]bool{
		"utun8": true, "tun0": true, "wg0": true, "CloudflareWARP": true, "ppp0": true,
		"cscotun0": true, "proton0": true, "mullvad-wg": true,
		"en0": false, "eth0": false, "wlan0": false, "bridge100": false, "docker0": false,
	} {
		if got := IsTunnelName(name); got != want {
			t.Errorf("IsTunnelName(%q) = %v, want %v", name, got, want)
		}
	}
}
