package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/siteops"
	lerdSystemd "github.com/geodro/lerd/internal/systemd"
	"github.com/spf13/cobra"
)

// NewSecureCmd returns the secure command.
func NewSecureCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "secure [name]",
		Short: "Enable HTTPS for the current site using mkcert (cert SANs cover *.<branch>.<site>.test for worktrees)",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runSecure,
	}
}

// NewUnsecureCmd returns the unsecure command.
func NewUnsecureCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unsecure [name]",
		Short: "Disable HTTPS for the current site",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runUnsecure,
	}
}

func resolveSiteName(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// Look up by path first so directory names like "astrolov.com" resolve
	// correctly to their registered site name (e.g. "astrolov").
	if site, err := config.FindSiteByPath(cwd); err == nil {
		return site.Name, nil
	}
	return filepath.Base(cwd), nil
}

func runSecure(_ *cobra.Command, args []string) error {
	return toggleSecureCmd(args, true)
}

func runUnsecure(_ *cobra.Command, args []string) error {
	return toggleSecureCmd(args, false)
}

// toggleSecureCmd is the CLI entry-point shared by `lerd secure` and
// `lerd unsecure`. It delegates the core flip to siteops.SetSecured (the
// single source of truth shared with the UI and MCP code paths) and
// supplies CLI-specific post-toggle hooks: Stripe listener restart and a
// best-effort lan:refresh notification to the daemon so any running LAN
// share proxy re-binds to the new backend port.
func toggleSecureCmd(args []string, secured bool) error {
	name, err := resolveSiteName(args)
	if err != nil {
		return err
	}
	site, err := config.FindSite(name)
	if err != nil {
		return fmt.Errorf("site %q not found — run 'lerd link' first", name)
	}
	if secured {
		if gcfg, _ := config.LoadGlobal(); !gcfg.DNSManaged() {
			return certs.ErrDNSDisabled
		}
	}
	verb := "enabling HTTPS"
	if !secured {
		verb = "disabling HTTPS"
	}
	feedback.Begin()
	step := feedback.Start(verb)
	if err := siteops.SetSecured(site, secured); err != nil {
		step.Fail(err)
		return err
	}
	scheme := "http"
	if secured {
		scheme = "https"
	}
	step.OK(feedback.Val(scheme + "://" + site.PrimaryDomain()))
	return nil
}

// RestartStripeIfActive is exported so the daemon's stripe:refresh HTTP
// handler can run the same Stripe restart logic as the CLI. SetSecured
// posts to that endpoint after every toggle, so this is the single
// implementation across CLI / UI / MCP.
func RestartStripeIfActive(site *config.Site) { restartStripeIfActive(site) }

// restartStripeIfActive restarts the Stripe listener for the site if it is currently running,
// so that --forward-to picks up the new http/https scheme.
func restartStripeIfActive(site *config.Site) {
	unitName := "lerd-stripe-" + site.Name
	if !lerdSystemd.IsServiceActive(unitName) {
		return
	}
	scheme := "http"
	if site.Secured {
		scheme = "https"
	}
	baseURL := scheme + "://" + site.PrimaryDomain()
	if err := StripeStartForSite(site.Name, site.Path, baseURL); err != nil {
		feedback.Warn("updating stripe listener unit: %v", err)
		return
	}
	if err := lerdSystemd.RestartService(unitName); err != nil {
		feedback.Warn("restarting stripe listener: %v", err)
		return
	}
	fmt.Printf("  Restarted stripe listener → %s%s\n", baseURL, config.StripeWebhookPath(site.Path))
}
