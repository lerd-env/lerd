package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewDNSEnableCmd returns the dns:enable command, which turns on lerd-managed
// DNS (dnsmasq + .test + HTTPS) after install and repairs an already-enabled
// but broken setup.
func NewDNSEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dns:enable",
		Short: "Let lerd manage DNS: dnsmasq, .test resolution and HTTPS (repairs if already on)",
		Args:  cobra.NoArgs,
		RunE:  runDNSEnable,
	}
}

// NewDNSDisableCmd returns the dns:disable command, which stops lerd-managed
// DNS, tears down dnsmasq, and moves sites to *.localhost plain HTTP.
func NewDNSDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dns:disable",
		Short: "Stop managing DNS: tear down dnsmasq, sites fall back to *.localhost (no HTTPS)",
		Args:  cobra.NoArgs,
		RunE:  runDNSDisable,
	}
}

// NewDNSRepairCmd returns the dns:repair command, which re-runs the DNS setup
// (mkcert CA, dnsmasq config, sudoers, resolver, container) to fix a broken but
// enabled DNS without changing the mode.
func NewDNSRepairCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dns:repair",
		Short: "Re-run DNS setup to fix a broken but enabled .test resolution",
		Args:  cobra.NoArgs,
		RunE:  runDNSRepair,
	}
}

func runDNSEnable(_ *cobra.Command, _ []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return fmt.Errorf("loading config: %w", err)
	}
	feedback.Begin()
	if cfg.DNS.Enabled {
		feedback.Line("lerd DNS is already enabled, repairing the setup")
		dns.ForgetSudoersMarker()
		return reexecInstallReconcile()
	}
	newTLD := applyDNSTLDMigration(cfg.DNS.TLD, true)
	cfg.DNS.Enabled = true
	cfg.DNS.TLD = newTLD
	if err := config.SaveGlobal(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	feedback.Line("enabling lerd-managed DNS")
	return reexecInstallReconcile()
}

func runDNSDisable(_ *cobra.Command, _ []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return fmt.Errorf("loading config: %w", err)
	}
	feedback.Begin()
	if !cfg.DNS.Enabled {
		feedback.Line("lerd DNS is already disabled")
		return nil
	}
	newTLD := applyDNSTLDMigration(cfg.DNS.TLD, false)
	cfg.DNS.Enabled = false
	cfg.DNS.TLD = newTLD
	if err := config.SaveGlobal(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	feedback.Line("disabling lerd-managed DNS")
	return reexecInstallReconcile()
}

func runDNSRepair(_ *cobra.Command, _ []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if !cfg.DNS.Enabled {
		return fmt.Errorf("DNS is disabled; enable it first with `lerd dns:enable`")
	}
	feedback.Begin()
	feedback.Line("repairing lerd-managed DNS")
	// Force the sudoers drop-in to be rewritten: repair exists to restore a
	// broken setup, and the content marker alone can't tell a deleted drop-in
	// from an up-to-date one.
	dns.ForgetSudoersMarker()
	return reexecInstallReconcile()
}

// toggledCanonicalTLD returns the TLD to use when flipping DNS management to
// enabling. It swaps only the canonical default (localhost <-> test) so a custom
// TLD the user set in config.yaml is preserved. Called on a known transition.
func toggledCanonicalTLD(prevTLD string, enabling bool) string {
	switch {
	case enabling && prevTLD == "localhost":
		return "test"
	case !enabling && prevTLD == "test":
		return "localhost"
	}
	return prevTLD
}

// confirmTLDRewrite is the prompt seam for the TLD-rewrite decision, overridden
// in tests to exercise the accept and decline branches without a terminal.
var confirmTLDRewrite = confirmInstallPromptDefault

// applyDNSTLDMigration computes the new canonical TLD for a mode flip and, when
// it changes and sites still carry the old TLD, offers to rewrite their domains,
// .env APP_URL and vhosts (mirroring the installer's TLD-migration prompt).
// Disabling forces sites unsecure since HTTPS is unavailable on .localhost.
// Returns the TLD to persist; the flip is recorded even if the rewrite is
// declined so new sites land on the right TLD.
func applyDNSTLDMigration(prevTLD string, enabling bool) string {
	if prevTLD == "" {
		prevTLD = "test"
	}
	newTLD := toggledCanonicalTLD(prevTLD, enabling)
	if newTLD != prevTLD {
		if affected := sitesWithTLD(prevTLD); len(affected) > 0 {
			feedback.Line(fmt.Sprintf("TLD change: %d site(s) currently on .%s -> .%s", len(affected), prevTLD, newTLD))
			feedback.Note(strings.Join(affected, ", "))
			if confirmTLDRewrite(fmt.Sprintf("Rewrite domains, .env APP_URL, and vhosts to .%s?", newTLD), true) {
				migrateSiteTLD(prevTLD, newTLD, !enabling)
			} else {
				feedback.Note("skipped, sites still reference ." + prevTLD)
				// Domains stay on the old TLD, but HTTPS tracks DNS: unsecure them
				// on disable (restore on enable) so a declined rewrite never leaves
				// an HTTPS-only vhost the torn-down resolver can no longer reach.
				adjustSitesSecuredForDNS(prevTLD, enabling)
			}
		}
		return newTLD
	}
	// No canonical rename (a preserved custom TLD): domains stay, but HTTPS still
	// tracks DNS, so a disabled custom-TLD site doesn't keep an HTTPS-only vhost
	// nginx serves after the cert/resolver layer is gone.
	adjustSitesSecuredForDNS(prevTLD, enabling)
	return prevTLD
}

// reexecInstallReconcile re-execs `lerd install --from-update`, the idempotent
// reconcile that sets up or tears down DNS to match the saved choice (and
// repairs a broken enabled setup). Mirrors how `lerd update` applies changes.
func reexecInstallReconcile() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating lerd binary: %w", err)
	}
	c := exec.Command(self, "install", "--from-update")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}
