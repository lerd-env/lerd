// Package lanip finds the host's LAN IPv4 address. It is a leaf package
// (stdlib only) so dns, podman, tui and ui can share it without a cycle.
package lanip

import (
	"net"
	"strings"
)

// Primary returns the IPv4 the kernel would use to reach a public address,
// unless that is a VPN tunnel (WARP, WireGuard) the LAN cannot reach: then the
// first private address on a physical interface. The dial sends no packets.
func Primary() string {
	ifaces, _ := net.Interfaces()
	if ip := dialProbe(); ip != nil && !IsTunnelAddr(ifaces, ip) {
		return ip.String()
	}
	return interfaceProbe(ifaces)
}

// IsTunnelInterface reports whether the interface looks like a VPN tunnel
// (macOS utun*, Linux tun*/tap*, WireGuard wg*, etc.).
func IsTunnelInterface(name string) bool {
	for _, prefix := range []string{"utun", "tun", "tap", "wg", "ipsec", "ppp", "CloudflareWARP"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func isContainerInterface(name string) bool {
	for _, prefix := range []string{"docker", "podman", "veth", "bridge", "br-", "vf-", "vz-"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func dialProbe() net.IP {
	conn, err := net.Dial("udp4", "1.1.1.1:80")
	if err != nil {
		return nil
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr == nil {
		return nil
	}
	return addr.IP
}

// IsTunnelAddr reports whether ip is assigned to a VPN tunnel interface.
func IsTunnelAddr(ifaces []net.Interface, ip net.IP) bool {
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.Equal(ip) {
				return IsTunnelInterface(iface.Name)
			}
		}
	}
	return false
}

// interfaceProbe prefers a private address on a physical interface, then any
// non-loopback IPv4 on a non-tunnel interface.
func interfaceProbe(ifaces []net.Interface) string {
	var fallback string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || IsTunnelInterface(iface.Name) {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			v4 := ipnet.IP.To4()
			if v4 == nil || v4.IsLoopback() {
				continue
			}
			if v4.IsPrivate() && !isContainerInterface(iface.Name) {
				return v4.String()
			}
			if fallback == "" {
				fallback = v4.String()
			}
		}
	}
	return fallback
}
