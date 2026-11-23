// Package pfredirect is the pf rule lerd used on macOS before the launchd port
// forward, redirecting loopback 80 and 443 to nginx's high loopback port. Start
// re-loads it until lerd install replaces it, and the forward reuses its probe.
package pfredirect

import (
	"errors"
	"fmt"
	"io"
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
// nginx, a reply on from counts: a held socket alone proves nothing.
func loopbackReaches(from, to int) bool {
	ln, err := net.Listen("tcp", loopbackAddr(to))
	if err != nil {
		return answers(from)
	}
	defer ln.Close()
	conn, err := net.DialTimeout("tcp", loopbackAddr(from), probeTimeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	token := fmt.Sprintf("lerd-probe-%d", time.Now().UnixNano())
	if _, err := conn.Write([]byte(token)); err != nil {
		return false
	}
	return acceptsOwn(ln.(*net.TCPListener), token, time.Now().Add(probeTimeout))
}

// answers reports whether anything on port replies to a request. nginx answers
// even plain HTTP sent to its TLS port.
func answers(port int) bool {
	conn, err := net.DialTimeout("tcp", loopbackAddr(port), probeTimeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(probeTimeout)) //nolint:errcheck
	if _, err := conn.Write([]byte("HEAD / HTTP/1.0\r\n\r\n")); err != nil {
		return false
	}
	_, err = conn.Read(make([]byte, 1))
	return err == nil
}

// acceptsOwn reports whether the connection carrying token arrives on ln
// before the deadline. A browser retrying through a working redirect can land
// first, and a relay hides the probe's address, so the token identifies it.
func acceptsOwn(ln *net.TCPListener, token string, deadline time.Time) bool {
	if err := ln.SetDeadline(deadline); err != nil {
		return false
	}
	for {
		got, err := ln.Accept()
		if err != nil {
			return false
		}
		got.SetReadDeadline(time.Now().Add(probeTimeout / 4)) //nolint:errcheck
		buf := make([]byte, len(token))
		_, err = io.ReadFull(got, buf)
		got.Close()
		if err == nil && string(buf) == token {
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

const skipCheck = "sudo pfctl -s Interfaces -i lo0 -v"
