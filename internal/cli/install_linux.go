//go:build linux

package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/phpantom"
)

func downloadBinaries(w io.Writer) error {
	binDir := config.BinDir()
	var pins pinnedTools

	// composer
	composerPharPath := filepath.Join(binDir, "composer.phar")
	if _, err := os.Stat(composerPharPath); os.IsNotExist(err) {
		if err := replaceTool(&pins, "composer", composerPharPath, w); err != nil {
			return fmt.Errorf("composer download: %w", err)
		}
	}

	// fnm — skipped when the user drives Node via their own nvm, since lerd never
	// provisions nvm and fnm would sit unused.
	// Switching back with `lerd node:manager fnm` calls ensureFnmBinary on demand.
	cfg, _ := config.LoadGlobal()
	if cfg == nil || cfg.NodeManager() != "nvm" {
		if err := ensureFnmBinary(w); err != nil {
			return err
		}
	}

	// mkcert
	mkcertPath := certs.MkcertPath()
	if _, err := os.Stat(mkcertPath); os.IsNotExist(err) {
		if err := replaceTool(&pins, "mkcert", mkcertPath, w); err != nil {
			return fmt.Errorf("mkcert download: %w", err)
		}
	}

	// phpantom_lsp powers tinker autocomplete in the web UI. Best-effort:
	// the UI also fetches it lazily on first tinker connect, so a failure
	// here (offline install, unsupported arch) must not abort setup.
	if !phpantom.Installed() {
		if err := phpantom.EnsureBinary(context.Background(), w); err != nil {
			fmt.Fprintf(w, "      Warning: phpantom_lsp download failed (%v); tinker autocomplete loads on first use instead\n", err)
		}
	}

	return nil
}

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

// ensureMkcertCA generates the root CA and gets it trusted. Generation and the
// browser NSS store are per-user and need no root; the system trust store is
// done by re-executing `lerd bootstrap --trust-ca`, so the CA reaches the same
// place by the same code on both install routes. An unattended run stops after
// generation because its maintainer script runs the trust pass itself.
func ensureMkcertCA(unattended bool) {
	gen := exec.Command(certs.MkcertPath(), "-install")
	gen.Env = append(os.Environ(), "TRUST_STORES=nss")
	gen.Stdout, gen.Stderr = io.Discard, io.Discard
	gen.Run() //nolint:errcheck

	if unattended || certs.CATrusted() {
		return
	}
	args := []string{"bootstrap", "--trust-ca"}
	if root, err := certs.CARoot(); err == nil && root != "" {
		args = append(args, "--ca-root", root)
	}
	feedback.Sudo("Trusting the mkcert CA in the system store")
	if err := sudoSelfRunner(args...); err != nil {
		feedback.Warn("trusting the mkcert CA: %v", err)
	}
}

// removeSystemTrustAnchor drops the CA that ensureMkcertCA installed. mkcert's
// own -uninstall keys on the filename it wrote, so it never removes this one.
func removeSystemTrustAnchor() {
	if !certs.SystemTrustAnchorPresent() {
		return
	}
	feedback.Sudo("Removing the mkcert CA from the system trust store")
	if err := sudoSelfRunner("bootstrap", "--untrust-ca"); err != nil {
		feedback.Warn("removing the mkcert CA: %v", err)
	}
}
