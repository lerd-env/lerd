// Package pfredirect keeps nginx's privileged ports on loopback on macOS.
// gvproxy runs as the user and can only publish a port below 1024 on every
// interface, so nginx publishes on a high loopback port instead and a pf rule
// redirects loopback 80 and 443 to it.
package pfredirect

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// Anchor sits under com.apple/*, which the stock /etc/pf.conf already loads,
// so lerd never edits pf.conf itself.
const Anchor = "com.apple/lerd"

const pfctl = "/sbin/pfctl"

// AnchorPath is a var so tests can point it at a fixture.
var AnchorPath = "/etc/pf.anchors/lerd"

// Target is the loopback port a redirected port is published on.
func Target(port int) int { return port + 17000 }

// Ports lists nginx's host ports that need the redirect, the ones below 1024.
func Ports() []int {
	httpPort, httpsPort := config.NginxPorts()
	var ports []int
	for _, p := range []int{httpPort, httpsPort} {
		if p > 0 && p < 1024 {
			ports = append(ports, p)
		}
	}
	return ports
}

// Render returns the anchor rules redirecting each port to its Target.
func Render(ports []int) string {
	var b strings.Builder
	b.WriteString("# Managed by lerd, keeps lerd-nginx on loopback.\n")
	for _, p := range ports {
		fmt.Fprintf(&b, "rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port %d -> 127.0.0.1 port %d\n", p, Target(p))
	}
	return b.String()
}

// Redirected returns the ports the installed anchor covers. It is empty when
// the anchor is missing or was rendered for other ports, so nginx keeps its
// wide bind rather than moving to a port nothing forwards to.
func Redirected() map[int]bool {
	ports := Ports()
	got, err := os.ReadFile(AnchorPath)
	if len(ports) == 0 || err != nil || string(got) != Render(ports) {
		return nil
	}
	set := make(map[int]bool, len(ports))
	for _, p := range ports {
		set[p] = true
	}
	return set
}

// ErrNotEffective means pf took the anchor but loopback connections bypass it.
// Install has already removed the anchor by then, so lerd install brings it back.
var ErrNotEffective = errors.New("pf loaded the redirect but loopback connections bypass it; if " + skipCheck + " shows lo0 (skip), run sudo " + strings.Join(ReloadArgs(), " ") + ", which also drops rules a VPN or firewall loaded, then lerd install")

// reaches is a var so tests can stand in for pf.
var reaches = loopbackReaches

const probeTimeout = time.Second

// Effective reports whether loopback connections to every redirected port
// arrive on its Target.
func Effective() bool {
	for p := range Redirected() {
		if !Reaches(p) {
			return false
		}
	}
	return true
}

// Reaches reports whether a loopback connection to port arrives on its Target.
func Reaches(port int) bool { return reaches(port, Target(port)) }

// loopbackReaches listens on to itself while it is free, so nginx need not be
// up and a wide bind on from cannot pass for the redirect. With to taken, by
// nginx, any answer on from counts.
func loopbackReaches(from, to int) bool {
	ln, err := net.Listen("tcp", loopbackAddr(to))
	if err != nil {
		conn, err := net.DialTimeout("tcp", loopbackAddr(from), probeTimeout)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}
	defer ln.Close()
	conn, err := net.DialTimeout("tcp", loopbackAddr(from), probeTimeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	return acceptsOwn(ln.(*net.TCPListener), conn.LocalAddr().String(), time.Now().Add(probeTimeout))
}

// acceptsOwn reports whether the connection from local arrives on ln before
// the deadline. A browser retrying through a working redirect can land first.
func acceptsOwn(ln *net.TCPListener, local string, deadline time.Time) bool {
	if err := ln.SetDeadline(deadline); err != nil {
		return false
	}
	for {
		got, err := ln.Accept()
		if err != nil {
			return false
		}
		own := got.RemoteAddr().String() == local
		got.Close()
		if own {
			return true
		}
	}
}

func loopbackAddr(port int) string { return fmt.Sprintf("127.0.0.1:%d", port) }

// SudoersPath is its own drop-in so the grant exists whether or not lerd
// manages DNS, whose drop-in install skips when DNS is off.
var SudoersPath = "/etc/sudoers.d/lerd-pf"

// ApplyArgs is the pfctl call that loads the anchor and enables pf.
func ApplyArgs() []string {
	return []string{pfctl, "-E", "-a", Anchor, "-f", AnchorPath}
}

// ReloadArgs reloads the main ruleset, dropping a skip on lo0 another tool
// set. It drops the anchor's rules too, so ApplyArgs has to follow it.
func ReloadArgs() []string {
	return []string{pfctl, "-f", "/etc/pf.conf"}
}

// Repair puts the redirect back after a reboot dropped it, and only reloads the
// main ruleset, which also drops a VPN's or firewall's rules, when lo0 is skipped.
func Repair() string {
	return "sudo " + strings.Join(ApplyArgs(), " ") + ", and if " + skipCheck + " shows lo0 (skip), sudo " + strings.Join(ReloadArgs(), " ") + " before it, which also drops rules a VPN or firewall loaded"
}

const skipCheck = "sudo pfctl -s Interfaces -i lo0 -v"

// Sudoers grants user exactly ApplyArgs without a password, so `lerd start`
// can re-load the anchor after a reboot drops it.
func Sudoers(user string) string {
	return fmt.Sprintf("# Lerd: re-loads the loopback redirect for lerd-nginx on start.\n%s ALL=(root) NOPASSWD: %s\n",
		user, strings.Join(ApplyArgs(), " "))
}
