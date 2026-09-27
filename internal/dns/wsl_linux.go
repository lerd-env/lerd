package dns

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/wsl"
)

// wslUpstreamDropin keeps WSL's DNS proxy as systemd-resolved's upstream. It is
// named for WSL rather than lerd on purpose: uninstall must leave it behind, or
// the distro is left with a resolver that has no upstream at all.
const wslUpstreamDropin = "/etc/systemd/resolved.conf.d/wsl-upstream.conf"

// Seams for tests.
var (
	isWSL             = wsl.IsWSL
	resolvConfPath    = "/etc/resolv.conf"
	resolvedIsRunning = func() bool {
		return exec.Command("systemctl", "is-active", "--quiet", "systemd-resolved").Run() == nil
	}
)

// wslHandoverNameservers returns WSL's DNS proxy addresses when resolv.conf
// still needs handing to systemd-resolved, nil when there is nothing to do.
func wslHandoverNameservers() []string {
	if !isWSL() || !resolvedIsRunning() {
		return nil
	}
	b, err := os.ReadFile(resolvConfPath)
	if err != nil {
		return nil
	}
	return wsl.GeneratedNameservers(string(b))
}

// handResolvConfToResolved points resolv.conf at systemd-resolved's stub on a WSL
// distro. WSL writes its own resolv.conf aimed straight at its DNS proxy, which
// bypasses resolved and every .test route lerd gives it, so the usual resolved
// path finds nothing to hook into. WSL's proxy stays the upstream, and
// generateResolvConf=false stops WSL rewriting the file on the next boot.
func handResolvConfToResolved() error {
	ns := wslHandoverNameservers()
	if ns == nil {
		return nil
	}
	feedback.Sudo("Handing WSL's resolv.conf to systemd-resolved so ." + ConfiguredTLD() + " can resolve")
	if err := sudoWriteFile(wslUpstreamDropin, []byte(wsl.ResolvedUpstreamDropin(ns)), 0644); err != nil {
		return err
	}
	cur, _ := os.ReadFile("/etc/wsl.conf")
	if next, changed := wsl.EnsureSectionLine(string(cur), "network", "generateResolvConf", "generateResolvConf=false"); changed {
		if err := sudoWriteFile("/etc/wsl.conf", []byte(next), 0644); err != nil {
			return err
		}
	}
	for _, args := range [][]string{
		{"ln", "-sf", "/run/systemd/resolve/stub-resolv.conf", resolvConfPath},
		{"systemctl", "restart", "systemd-resolved"},
	} {
		if out, err := exec.Command("sudo", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w: %s", args, err, out)
		}
	}
	return nil
}
