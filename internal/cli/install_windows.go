//go:build windows

package cli

import (
	"io"
	"os"
	"os/exec"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/feedback"
)

// runSystemSetup has nothing to apply on Windows: there is no port sysctl, no
// linger, and no root-level pass to route through. Privileged ports are not a
// concept here, and the DNS rule is written by ConfigureResolver.
func runSystemSetup(_ bool) error { return nil }

// ensureResolverSudoers keeps the shared install sequence intact; the Windows
// InstallSudoers is a no-op because there is no sudoers.
func ensureResolverSudoers() {
	dns.InstallSudoers() //nolint:errcheck
}

// ensureMkcertCA installs the mkcert root CA into the current user's trust
// store. Windows shows its own confirmation dialog the first time, so an
// unattended run skips the trust step instead of blocking on a prompt nobody
// can answer.
func ensureMkcertCA(unattended bool) {
	if unattended {
		return
	}
	cmd := exec.Command(certs.MkcertPath(), "-install")
	if certs.CATrusted() {
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	} else {
		feedback.Line("Installing mkcert CA (Windows will ask you to confirm)")
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	}
	cmd.Run() //nolint:errcheck
}

// removeSystemTrustAnchor is a no-op: mkcert -uninstall removes its own entry
// from the user store and nothing here writes a separate anchor.
func removeSystemTrustAnchor() {}

// removePortDropIn is a no-op, there is no port sysctl to undo.
func removePortDropIn() {}

// ensurePortForwarding is a no-op: the rootful Podman machine publishes 80 and
// 443 on the host directly, with no helper daemon like macOS needs.
func ensurePortForwarding() error { return nil }
