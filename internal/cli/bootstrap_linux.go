//go:build linux

package cli

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/feedback"
)

// runBootstrapSystem performs the root prerequisites a later unattended install
// relies on: the unprivileged-port sysctl, systemd linger, and the DNS sudoers
// rule that lets the install configure the resolver without prompting.
func runBootstrapSystem(target string) error {
	feedback.Header("Bootstrapping system for lerd")

	if err := writePortDropIn(unprivPortDropIn, execRunner); err != nil {
		feedback.Warn("enabling unprivileged ports: %v", err)
	} else {
		feedback.Done("unprivileged ports enabled for 80/443")
	}

	if target == "" {
		return nil
	}
	if err := enableLinger(target, execRunner); err != nil {
		feedback.Warn("enabling linger for %s: %v", target, err)
	} else {
		feedback.Done("systemd linger enabled for " + target)
	}
	if err := dns.WriteSudoersForUser(target); err != nil {
		feedback.Warn("installing DNS sudoers rule: %v", err)
	} else {
		feedback.Done("DNS sudoers rule installed for " + target)
	}
	return nil
}

// runBootstrapTrustCA installs the user-generated mkcert root CA into the system
// trust store, the one managed-DNS step the per-user install cannot do without
// an interactive sudo. Run after `lerd install --unattended` has generated the
// CA. A missing CA is not an error (localhost-mode installs have none).
func runBootstrapTrustCA(target string) error {
	if target == "" {
		return fmt.Errorf("--trust-ca needs a target user (pass --user)")
	}
	u, err := user.Lookup(target)
	if err != nil {
		return fmt.Errorf("looking up user %s: %w", target, err)
	}
	caPath := filepath.Join(u.HomeDir, ".local", "share", "mkcert", "rootCA.pem")
	pem, err := os.ReadFile(caPath)
	if err != nil {
		feedback.Note("no mkcert CA at " + caPath + " yet, skipping system trust")
		return nil
	}
	if err := certs.TrustCAInSystemStore(pem); err != nil {
		return fmt.Errorf("trusting CA in system store: %w", err)
	}
	feedback.Done("mkcert CA trusted in the system store")
	return nil
}
