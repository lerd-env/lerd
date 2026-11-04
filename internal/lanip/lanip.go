// Package lanip finds the host's LAN address. It is a leaf package (stdlib
// only) so dns, podman, tui and ui can share it without a cycle.
package lanip

import (
	"net"
	"strings"
)

// tunnelPrefixes are interface-name prefixes used by VPN tunnels: OpenVPN and
// WireGuard tun*/wg*, macOS utun*, tap*, IPsec, ppp, Cisco AnyConnect
// cscotun*, ProtonVPN proton*, Mullvad mullvad*, Cloudflare WARP on Linux.
var tunnelPrefixes = []string{"tun", "utun", "tap", "wg", "ipsec", "ppp", "cscotun", "proton", "mullvad", "CloudflareWARP"}

var containerPrefixes = []string{"docker", "podman", "veth", "bridge", "br-", "vf-", "vz-"}

// DummyIface is lerd's always-up link for offline .test resolution. Its
// 192.0.2.1 is a placeholder only this host can use, so it is never picked.
const DummyIface = "lerd0"

// nic is the part of a host interface the address pick looks at.
type nic struct {
	name  string
	flags net.Flags
	ips   []net.IP
}

// Primary returns the IPv4 the kernel would use to reach a public address,
// unless that is a VPN tunnel (WARP, WireGuard) the LAN cannot reach: then the
// first private address on a physical interface. The dial sends no packets.
func Primary() string {
	return pickV4(dialProbe("udp4", "1.1.1.1:80"), hostNICs())
}

// PrimaryV6 returns the host's primary global-unicast IPv6 off any VPN
// tunnel, or "". Link-local and ULA are skipped: the LAN must reach it.
func PrimaryV6() string {
	return pickV6(dialProbe("udp6", "[2001:4860:4860::8888]:80"), hostNICs())
}

// IsTunnelName reports whether an interface name belongs to a VPN tunnel.
func IsTunnelName(name string) bool {
	return hasPrefix(name, tunnelPrefixes)
}

// isTunnel also catches branded clients with custom names through the
// point-to-point flag every tunnel carries.
func isTunnel(n nic) bool {
	return IsTunnelName(n.name) || (n.flags&net.FlagPointToPoint != 0 && n.flags&net.FlagLoopback == 0)
}

func hasPrefix(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func pickV4(dialed net.IP, nics []nic) string {
	if v4 := dialed.To4(); v4 != nil && !onTunnel(dialed, nics) {
		return v4.String()
	}
	var fallback string
	for _, n := range usable(nics) {
		for _, ip := range n.ips {
			v4 := ip.To4()
			if v4 == nil || v4.IsLoopback() {
				continue
			}
			if v4.IsPrivate() && !hasPrefix(n.name, containerPrefixes) {
				return v4.String()
			}
			if fallback == "" {
				fallback = v4.String()
			}
		}
	}
	return fallback
}

func pickV6(dialed net.IP, nics []nic) string {
	if globalV6(dialed) && !onTunnel(dialed, nics) {
		return dialed.String()
	}
	for _, n := range usable(nics) {
		for _, ip := range n.ips {
			if globalV6(ip) {
				return ip.String()
			}
		}
	}
	return ""
}

func globalV6(ip net.IP) bool {
	return ip != nil && ip.To4() == nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast()
}

// usable drops interfaces that are down, loopback, a VPN tunnel or lerd's dummy.
func usable(nics []nic) []nic {
	var out []nic
	for _, n := range nics {
		if n.flags&net.FlagUp != 0 && n.flags&net.FlagLoopback == 0 && n.name != DummyIface && !isTunnel(n) {
			out = append(out, n)
		}
	}
	return out
}

func onTunnel(ip net.IP, nics []nic) bool {
	for _, n := range nics {
		for _, own := range n.ips {
			if own.Equal(ip) {
				return isTunnel(n)
			}
		}
	}
	return false
}

func dialProbe(network, addr string) net.IP {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil
	}
	defer conn.Close()
	udp, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || udp == nil {
		return nil
	}
	return udp.IP
}

func hostNICs() []nic {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	nics := make([]nic, 0, len(ifaces))
	for _, iface := range ifaces {
		n := nic{name: iface.Name, flags: iface.Flags}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				n.ips = append(n.ips, ipnet.IP)
			}
		}
		nics = append(nics, n)
	}
	return nics
}
