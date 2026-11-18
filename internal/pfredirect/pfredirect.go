// Package pfredirect keeps nginx's privileged ports on loopback on macOS.
// gvproxy runs as the user and can only publish a port below 1024 on every
// interface, so nginx publishes on a high loopback port instead and a pf rule
// redirects loopback 80 and 443 to it.
package pfredirect

import (
	"fmt"
	"os"
	"strings"

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

// SudoersPath is its own drop-in so the grant exists whether or not lerd
// manages DNS, whose drop-in install skips when DNS is off.
var SudoersPath = "/etc/sudoers.d/lerd-pf"

// ApplyArgs is the pfctl call that loads the anchor and enables pf.
func ApplyArgs() []string {
	return []string{pfctl, "-E", "-a", Anchor, "-f", AnchorPath}
}

// Sudoers grants user exactly ApplyArgs without a password, so `lerd start`
// can re-load the anchor after a reboot drops it.
func Sudoers(user string) string {
	return fmt.Sprintf("# Lerd: re-loads the loopback redirect for lerd-nginx on start.\n%s ALL=(root) NOPASSWD: %s\n",
		user, strings.Join(ApplyArgs(), " "))
}
