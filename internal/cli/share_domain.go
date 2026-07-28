package cli

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewShareDomainCmd returns the share:domain command.
func NewShareDomainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "share:domain [domain|none]",
		Short: "Show or set the base domain public shares are served under",
		Long: `Without an argument, prints the base domain "lerd share" serves a site under.

With a domain, every Cloudflare share is served on "<site>.<domain>" through a
named tunnel, so the public URL stays the same between runs instead of being a
fresh trycloudflare.com address each time. The domain's DNS must be managed by
Cloudflare, and cloudflared must be authorized once with "cloudflared tunnel login".

"none" forgets the setting, so quick tunnels come back and the dashboard asks
again the next time a site is shared through Cloudflare.

"lerd share --domain" still wins for a single run.`,
		Example: `  lerd share:domain
  lerd share:domain example.com
  lerd share:domain none`,
		Args: cobra.MaximumNArgs(1),
		RunE: runShareDomain,
	}
}

func runShareDomain(_ *cobra.Command, args []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		switch {
		case cfg.Share.BaseDomain != "":
			fmt.Printf("%s (a site is shared on <site>.%s)\n", cfg.Share.BaseDomain, cfg.Share.BaseDomain)
		case cfg.Share.BaseDomainAnswered:
			fmt.Println("none (quick tunnels, and the dashboard no longer asks)")
		default:
			fmt.Println("none (quick tunnels)")
		}
		fmt.Println("\nChange it with: lerd share:domain example.com|none")
		return nil
	}

	if strings.EqualFold(args[0], "none") {
		if err := SetShareBaseDomain("", false); err != nil {
			return err
		}
		feedback.Begin()
		feedback.Done("share base domain cleared, back to " + feedback.Val("quick tunnels"))
		return nil
	}

	domain, err := normalizeBaseDomain(args[0])
	if err != nil {
		return err
	}
	if err := SetShareBaseDomain(domain, true); err != nil {
		return err
	}
	feedback.Begin()
	feedback.Done("share base domain set to " + feedback.Val(domain))
	return nil
}
