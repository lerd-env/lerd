//go:build linux

package cli

import (
	"os"
	"os/exec"
	"strings"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/feedback"
)

// ensurePortForwarding is a no-op on Linux; the system setup pass handles
// port 80/443 access via the ip_unprivileged_port_start sysctl.
func ensurePortForwarding() error { return nil }

// Seams for the root pass, so tests can drive each precondition and observe the
// escalation without a real kernel, login manager, or sudo.
var (
	unprivPortsNeeded = defaultUnprivPortsNeeded
	lingerNeeded      = defaultLingerNeeded
	dnsSudoersReady   = dns.SudoersCurrent
	recordDNSSudoers  = dns.RecordSudoersForUser
	sudoSelfRunner    = defaultSudoSelfRunner
	legacySystemSetup = defaultLegacySystemSetup
)

// defaultSudoSelfRunner re-executes this binary under sudo. The absolute path
// from os.Executable sidesteps sudo's secure_path, which does not carry
// ~/.local/bin.
func defaultSudoSelfRunner(args ...string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command("sudo", append([]string{self}, args...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// systemSetupNeeded reports whether any root-level step still has to run. An
// install where they all already apply must not ask for a password to do
// nothing, which is the common case on every reinstall and update.
func systemSetupNeeded(wantDNS bool) bool {
	if unprivPortsNeeded() || lingerNeeded() {
		return true
	}
	return wantDNS && !dnsSudoersReady()
}

// runSystemSetup applies the machine-global half of the install through a
// single `sudo lerd bootstrap --system`, the same entry point a package
// maintainer script calls, so both install routes share one implementation of
// the sysctl, linger and sudoers steps and ask for a password once. A host
// where the re-exec cannot run falls back to the individual steps.
func runSystemSetup(wantDNS bool) error {
	if !systemSetupNeeded(wantDNS) {
		return nil
	}
	args := []string{"bootstrap", "--system"}
	if !wantDNS {
		args = append(args, "--skip-sudoers")
	}
	feedback.Sudo("Applying system setup")
	err := sudoSelfRunner(args...)
	if err == nil {
		if wantDNS {
			recordDNSSudoers(currentUserName())
		}
		return nil
	}
	feedback.Warn("system setup through sudo failed (%v), applying the steps individually", err)
	return legacySystemSetup(wantDNS)
}

// defaultLegacySystemSetup is the pre-bootstrap path, kept as the fallback for
// hosts without a usable sudo. Each step prompts on its own.
func defaultLegacySystemSetup(wantDNS bool) error {
	if err := ensureUnprivilegedPorts(); err != nil {
		return err
	}
	if err := ensureSystemdLinger(); err != nil {
		feedback.Warn("%v", err)
	}
	if wantDNS {
		dns.InstallSudoers() //nolint:errcheck
	}
	return nil
}

// ensureResolverSudoers is a no-op on Linux: the grant carries no TLD, so the
// root pass installs it up front along with the other machine-global steps.
func ensureResolverSudoers() {}

// ensureCA generates the root CA and trusts it in the browser stores, then in
// the system store through `lerd bootstrap --trust-ca`, the code both install
// routes share. Unattended, the maintainer script runs that pass itself.
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
	args := []string{"bootstrap", "--trust-ca"}
	if root, err := certs.CARoot(); err == nil && root != "" {
		args = append(args, "--ca-root", root)
	}
	feedback.Sudo("Trusting the lerd CA in the system store")
	if err := sudoSelfRunner(args...); err != nil {
		feedback.Warn("trusting the lerd CA: %v", err)
	}
}

// removeSystemTrustAnchor drops the CA that ensureCA installed, and the one an
// older install let mkcert write under its own name.
func removeSystemTrustAnchor() {
	if !certs.SystemTrustAnchorPresent() {
		return
	}
	args := []string{"bootstrap", "--untrust-ca"}
	if root, err := certs.CARoot(); err == nil && root != "" {
		args = append(args, "--ca-root", root)
	}
	feedback.Sudo("Removing the lerd CA from the system trust store")
	if err := sudoSelfRunner(args...); err != nil {
		feedback.Warn("removing the lerd CA: %v", err)
	}
}

// applyLoopbackRedirect is a no-op on Linux, where a loopback bind holds.
func applyLoopbackRedirect() {}

// removeLoopbackRedirect is a no-op on Linux, see applyLoopbackRedirect.
func removeLoopbackRedirect() {}

// removePortDropIn drops the sysctl drop-in bootstrap wrote, so the lowered
// port start stops applying from the next boot. The live value is left alone
// because another tool may bind a low port under it right now.
func removePortDropIn() {
	if _, err := os.Stat(unprivPortDropIn); err != nil {
		return
	}
	feedback.Sudo("Removing the unprivileged port setting")
	if err := bootstrapRunner("sudo", "rm", "-f", unprivPortDropIn); err != nil {
		feedback.Warn("removing %s: %v", unprivPortDropIn, err)
		return
	}
	if initramfsHasPortDropIn() {
		feedback.Warn("the initramfs still applies the port setting at boot, rebuild it with: sudo dracut -f")
	}
}

// initramfsHasPortDropIn reports whether the running kernel's initramfs still
// carries the drop-in: dracut copies /etc/sysctl.d in and applies it at boot
// until the image is rebuilt. Only dracut ships lsinitrd, the other generators
// leave sysctl.d out.
var initramfsHasPortDropIn = func() bool {
	if _, err := exec.LookPath("lsinitrd"); err != nil {
		return false
	}
	out, _ := exec.Command("sudo", "lsinitrd", "-f", strings.TrimPrefix(unprivPortDropIn, "/")).Output()
	return strings.Contains(string(out), unprivPortSetting)
}
