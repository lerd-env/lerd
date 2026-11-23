//go:build darwin

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/pfredirect"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/portforward"
)

// runSystemSetup applies the root-level steps individually. macOS has no
// `lerd bootstrap` to route them through; the port sysctl and linger are
// no-ops here. The resolver grant is deliberately not part of this pass, see
// ensureResolverSudoers.
func runSystemSetup(_ bool) error {
	if err := ensureUnprivilegedPorts(); err != nil {
		return err
	}
	if err := ensureSystemdLinger(); err != nil {
		feedback.Warn("%v", err)
	}
	ensureLoopbackRedirect()
	return nil
}

// ensureLoopbackRedirect installs the launchd jobs that keep nginx's 80 and
// 443 on loopback; without them gvproxy can only publish them on every interface.
func ensureLoopbackRedirect() {
	user := os.Getenv("USER")
	if portforward.Pending(user) {
		pauseNginx()
		feedback.Sudo("Keeping nginx ports on loopback (launchd port forward)")
	}
	moved, err := portforward.Install(user, dns.SudoWriteFile)
	loopbackRedirectAdded = moved
	if err != nil {
		feedback.Warn("port forward for nginx, ports 80 and 443 stay reachable from the network: %v", err)
	}
}

// Vars so tests can stand in for podman.
var (
	nginxRunningFn = func() bool { r, _ := podman.ContainerRunning("lerd-nginx"); return r }
	stopNginxFn    = func() error { return podman.StopUnit("lerd-nginx") }
	nginxWideFn    = nginxPublishesWide
)

// pauseNginx stops a nginx holding 0.0.0.0:80, which launchd could not take
// then. Install starts it again once its unit is rewritten for the forward.
func pauseNginx() {
	if !nginxWideFn() {
		return
	}
	nginxPaused = nginxRunningFn()
	stopNginxFn() //nolint:errcheck // not running is fine
}

// nginxPublishesWide reports whether nginx's unit binds its ports on every
// interface: LAN exposed, or widened because nothing forwards loopback.
func nginxPublishesWide() bool {
	if portforward.Installed() {
		return false
	}
	if cfg, err := config.LoadGlobal(); err != nil || cfg == nil || cfg.LAN.Exposed {
		return true
	}
	wide, err := podman.UnitLoopbackPortsPublishedWide("lerd-nginx")
	return err != nil || len(wide) > 0
}

// applyLoopbackRedirect re-loads the legacy pf anchor, which a reboot drops,
// until lerd install replaces it with the launchd port forward.
func applyLoopbackRedirect() {
	if err := pfredirect.Apply(); err != nil {
		feedback.Warn("loading the pf redirect for nginx: %v", err)
	}
}

// removeLoopbackRedirect undoes ensureLoopbackRedirect.
func removeLoopbackRedirect() {
	if len(portforward.Redirected()) == 0 {
		if _, err := os.Stat(pfredirect.AnchorPath); err != nil {
			return
		}
	}
	feedback.Sudo("Removing the port forward for nginx")
	if err := portforward.Remove(); err != nil {
		feedback.Warn("removing the port forward: %v", err)
	}
}

// ensureResolverSudoers writes the passwordless grant once the DNS choice has
// been persisted. The macOS rule names /etc/resolver/<tld>, so rendering it
// before the TLD is saved would grant the previous TLD's path and leave the
// watcher prompting. The Linux grant carries no TLD and is applied earlier, by
// the root pass.
func ensureResolverSudoers() {
	dns.InstallSudoers() //nolint:errcheck
}

// ensureCA generates the root CA and trusts it in the browser stores and the
// system keychain. CATrusted reads trust settings, not the keychain item, so a
// CA whose trust a macOS update cleared gets the keychain write again.
func ensureCA(unattended bool) {
	if err := certs.EnsureCA(); err != nil {
		feedback.Warn("generating the lerd CA: %v", err)
		return
	}
	for _, err := range certs.InstallBrowserTrust() {
		feedback.Warn("trusting the lerd CA in a browser store, %v", err)
	}
	if unattended || certs.CATrusted() {
		return
	}
	feedback.Sudo("Trusting the lerd CA in the system keychain")
	// Named rather than swallowed: macOS draws the authorization dialog
	// itself, so a run with no window server behind it fails here.
	if err := certs.RepairSystemTrust(); err != nil {
		feedback.Warn("trusting the lerd CA: %v", err)
	}
}

// removeSystemTrustAnchor drops the keychain trust ensureCA set.
func removeSystemTrustAnchor() {
	if !certs.CATrusted() {
		return
	}
	feedback.Sudo("Removing the lerd CA from the system keychain")
	if err := certs.RemoveSystemTrust(); err != nil {
		feedback.Warn("removing the lerd CA: %v", err)
	}
}

// removePortDropIn is a no-op on macOS, which has no port sysctl to undo.
func removePortDropIn() {}

// ensurePortForwarding installs the podman-mac-helper, which allows Podman
// Machine to bind to privileged ports (80, 443) as a regular user on macOS.
//
// The helper ships with Podman (typically at /opt/homebrew/bin/podman-mac-helper
// or /usr/local/bin/podman-mac-helper) and installs a system LaunchDaemon via
// `sudo podman-mac-helper install`.
//
// If the helper is not found or already installed, this is a no-op.
func ensurePortForwarding() error {
	// Skip (and avoid the sudo prompt) if the LaunchDaemon is already installed.
	if podmanHelperInstalled("/Library/LaunchDaemons", currentUsername(os.Getenv("HOME"))) {
		return nil
	}

	// Locate the podman-mac-helper binary.
	helperPath, err := findPodmanMacHelper()
	if err != nil {
		feedback.Warn("podman-mac-helper not found — ports 80/443 may not work.")
		feedback.Note("Install Podman via Homebrew and re-run 'lerd install'.")
		return nil // not fatal; containers still work on non-privileged ports
	}

	feedback.Sudo("Installing podman-mac-helper for ports 80/443")
	cmd := exec.Command("sudo", helperPath, "install")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		feedback.Warn("podman-mac-helper install: %v", err)
		feedback.Note(fmt.Sprintf("Ports 80/443 may not work — run manually: sudo %s install", helperPath))
	}
	return nil
}

// podmanHelperInstalled reports whether the podman-mac-helper LaunchDaemon is
// already present. Recent Podman names the plist per-user
// (com.github.containers.podman.helper-<user>.plist); older releases used an
// unsuffixed name. Checking both lets us skip the sudo prompt when the helper
// is already installed.
func podmanHelperInstalled(daemonDir, username string) bool {
	candidates := []string{
		"com.github.containers.podman.helper-" + username + ".plist",
		"com.github.containers.podman.helper.plist",
	}
	for _, name := range candidates {
		if _, err := os.Stat(filepath.Join(daemonDir, name)); err == nil {
			return true
		}
	}
	return false
}

// findPodmanMacHelper returns the path to podman-mac-helper if found.
func findPodmanMacHelper() (string, error) {
	// Try PATH first (covers non-Homebrew installs).
	if p, err := exec.LookPath("podman-mac-helper"); err == nil {
		return p, nil
	}
	// Common Homebrew locations.
	for _, candidate := range []string{
		"/opt/homebrew/bin/podman-mac-helper",
		"/usr/local/bin/podman-mac-helper",
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("podman-mac-helper not found")
}
