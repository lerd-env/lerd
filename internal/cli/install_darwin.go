//go:build darwin

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

// runSystemSetup applies the root-level steps individually. macOS has no
// `lerd bootstrap` to route them through, and both of these are no-ops here:
// the port sysctl does not exist and there is no linger. The resolver grant is
// deliberately not part of this pass, see ensureResolverSudoers.
func runSystemSetup(_ bool) error {
	if err := ensureUnprivilegedPorts(); err != nil {
		return err
	}
	if err := ensureSystemdLinger(); err != nil {
		feedback.Warn("%v", err)
	}
	return nil
}

// ensureResolverSudoers writes the passwordless grant once the DNS choice has
// been persisted. The macOS rule names /etc/resolver/<tld>, so rendering it
// before the TLD is saved would grant the previous TLD's path and leave the
// watcher prompting. The Linux grant carries no TLD and is applied earlier, by
// the root pass.
func ensureResolverSudoers() {
	dns.InstallSudoers() //nolint:errcheck
}

// ensureMkcertCA installs the root CA. mkcert reaches the system keychain by
// itself here (prompting for the admin password via a GUI dialog the first
// time), so generation and trust are normally the same call.
//
// The one case that isn't: a reinstall where the CA cert is already present
// in the keychain but its trust settings were cleared independent of the
// cert item itself (a macOS update is the most common real trigger — see
// certs.CAPresentButUntrusted's doc comment). mkcert's own "already
// installed" self-check can't tell that state apart from a genuinely trusted
// CA — it verifies the self-signed root against itself, which succeeds
// cryptographically regardless of trust settings — so it would silently skip
// re-establishing trust exactly when that's what's needed. Repairing trust
// directly first, before mkcert gets a chance to (wrongly) decide there's
// nothing to do, is what actually fixes it.
func ensureMkcertCA(unattended bool) {
	if !unattended && certs.CAPresentButUntrusted() {
		feedback.Sudo("Repairing mkcert CA trust (present in the keychain but not trusted)")
		certs.RepairSystemTrust() //nolint:errcheck // best-effort; mkcert -install still runs next either way
	}
	cmd := exec.Command(certs.MkcertPath(), "-install")
	switch {
	case unattended:
		cmd.Env = append(os.Environ(), "TRUST_STORES=nss")
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	case certs.CATrusted():
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	default:
		feedback.Sudo("Installing mkcert CA")
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	}
	cmd.Run() //nolint:errcheck
}

// removeSystemTrustAnchor is a no-op on macOS: mkcert -uninstall removes its own
// keychain entry, and nothing here writes a separate anchor.
func removeSystemTrustAnchor() {}

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

	// fnm — macOS universal binary. Skipped when the user drives Node via their
	// own nvm, since lerd never provisions nvm and fnm would sit unused.
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
			feedback.WarnOn(w, "phpantom_lsp download failed (%v); tinker autocomplete loads on first use instead", err)
		}
	}

	return nil
}

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
