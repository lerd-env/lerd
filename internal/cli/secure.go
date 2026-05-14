package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
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
	verb := "Issuing certificate"
	if !secured {
		verb = "Removing certificate"
	}
	fmt.Printf("%s for %s...\n", verb, site.PrimaryDomain())

	if err := siteops.SetSecured(site, secured); err != nil {
		return err
	}
	scheme := "http"
	state := "Unsecured"
	if secured {
		scheme = "https"
		state = "Secured"
	}
	fmt.Printf("  Updated APP_URL=%s://%s and VITE_REVERB_* in .env\n", scheme, site.PrimaryDomain())
	fmt.Printf("%s: %s://%s\n", state, scheme, site.PrimaryDomain())
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
		fmt.Printf("[WARN] updating stripe listener unit: %v\n", err)
		return
	}
	if err := lerdSystemd.RestartService(unitName); err != nil {
		fmt.Printf("[WARN] restarting stripe listener: %v\n", err)
		return
	}
	fmt.Printf("  Restarted stripe listener → %s/stripe/webhook\n", baseURL)
}
