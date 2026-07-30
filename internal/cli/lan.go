package cli

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewLANCmd returns the `lerd lan` parent command. Site exposure and managed
// service exposure are separate persisted settings: sites follow
// cfg.LAN.Exposed, while databases, caches, and other managed services require
// both cfg.LAN.Exposed and cfg.LAN.ServicesExposed.
func NewLANCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lan",
		Short: "Expose lerd to other devices on the local network",
		Long: `Control whether lerd sites and managed services are reachable from
other devices on the local network.

By default every container PublishPort and the dashboard bind to loopback.
Run 'lerd lan:expose' to expose sites, DNS, and the dashboard listener on a
trusted LAN. Managed databases, caches, and other services remain loopback-only
unless you explicitly run 'lerd lan:services on'.`,
	}
	cmd.AddCommand(newLANExposeCmd())
	cmd.AddCommand(newLANUnexposeCmd())
	cmd.AddCommand(newLANStatusCmd())
	cmd.AddCommand(newLANServicesCmd())
	cmd.AddCommand(newLANShareCmd())
	cmd.AddCommand(newLANUnshareCmd())
	return cmd
}

// NewLANExposeCmd returns the `lerd lan:expose` colon-style alias.
func NewLANExposeCmd() *cobra.Command {
	cmd := newLANExposeCmd()
	cmd.Use = "lan:expose"
	cmd.Hidden = true
	return cmd
}

// NewLANUnexposeCmd returns the `lerd lan:unexpose` colon-style alias.
func NewLANUnexposeCmd() *cobra.Command {
	cmd := newLANUnexposeCmd()
	cmd.Use = "lan:unexpose"
	cmd.Hidden = true
	return cmd
}

// NewLANStatusCmd returns the `lerd lan:status` colon-style alias.
func NewLANStatusCmd() *cobra.Command {
	cmd := newLANStatusCmd()
	cmd.Use = "lan:status"
	cmd.Hidden = true
	return cmd
}

// NewLANShareCmd returns the `lerd lan:share` colon-style alias.
func NewLANShareCmd() *cobra.Command {
	cmd := newLANShareCmd()
	cmd.Use = "lan:share"
	return cmd
}

// NewLANUnshareCmd returns the `lerd lan:unshare` colon-style alias.
func NewLANUnshareCmd() *cobra.Command {
	cmd := newLANUnshareCmd()
	cmd.Use = "lan:unshare"
	return cmd
}

// NewLANServicesCmd returns the `lerd lan:services` colon-style command.
func NewLANServicesCmd() *cobra.Command {
	cmd := newLANServicesCmd()
	cmd.Use = "lan:services [on|off|status]"
	return cmd
}

func newLANExposeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "expose",
		Short: "Make lerd reachable from other devices on the local network",
		Long: `Exposes lerd sites on a trusted local network:

  - Rewrites lerd-nginx so ports 80 and 443 bind to the LAN.
  - Restarts nginx when its bind changes.
  - Rewrites dnsmasq to answer *.test with the host's LAN IP.
  - Starts the userspace DNS forwarder where the platform requires it.

Managed databases, caches, and other services stay loopback-only by default.
Run 'lerd lan:services on' once to include them. That preference persists and
applies automatically as services start, stop, or change ports.

The dashboard at port 7073 is gated by remote-control middleware. LAN clients
get 403 unless 'lerd remote-control on' has configured HTTP Basic auth.

The state persists in ~/.config/lerd/config.yaml. Re-running this command heals
drift between the config and installed runtime units.

Only use LAN exposure on a trusted network. Configure the host firewall for the
ports and devices that require access.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, _ := config.LoadGlobal()
			dnsOn := cfg == nil || cfg.DNS.Enabled
			// In disabled-DNS mode the dashboard is the only thing the LAN
			// exposure unlocks for remote devices, so bundle the
			// remote-control credential prompt into this single command if
			// it has not already been set.
			if !dnsOn && cfg != nil && cfg.UI.PasswordHash == "" {
				feedback.Line("DNS is disabled — the dashboard is the only thing reachable on the LAN, set HTTP Basic credentials so it is gated")
				if err := promptAndPersistRemoteControl(); err != nil {
					return err
				}
				cfg, _ = config.LoadGlobal()
			}
			feedback.Begin()
			expose := feedback.Start("exposing lerd on the LAN")
			lanIP, err := EnableLANExposure(func(step string) {
				feedback.Note(step)
			})
			if err != nil {
				expose.Fail(err)
				return err
			}
			expose.OK(feedback.Val(lanIP))
			if dnsOn {
				feedback.Note("sites: http://*.test (resolved via dnsmasq on " + lanIP + ":5300)")
			} else {
				feedback.Note("sites: only reachable via per-site `lerd lan:share` (no dnsmasq, *.localhost cannot resolve to a remote host)")
			}
			if cfg != nil && cfg.UI.PasswordHash != "" {
				feedback.Note(fmt.Sprintf("dashboard: http://%s:7073 (HTTP Basic auth required)", lanIP))
			} else {
				feedback.Note(fmt.Sprintf("dashboard: http://%s:7073 (LAN clients get 403 — run `lerd remote-control on` to grant LAN access)", lanIP))
			}
			if cfg != nil && cfg.LAN.ServicesExposed {
				feedback.Note("managed services: exposed on their configured host ports")
			} else {
				feedback.Note("managed services: loopback-only (run `lerd lan:services on` to expose them)")
			}
			if dnsOn {
				feedback.Note("allow ports 80, 443, 5300, 7073 through your firewall; `lerd remote-setup` generates a one-time bootstrap code")
			} else {
				feedback.Note("allow ports 80, 443, 7073 plus any `lerd lan:share` ports through your firewall")
			}
			return nil
		},
	}
}

func newLANUnexposeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unexpose",
		Short: "Restrict lerd to loopback only — safe for untrusted wifi",
		RunE: func(_ *cobra.Command, _ []string) error {
			feedback.Begin()
			restrict := feedback.Start("restricting lerd to loopback")
			if err := DisableLANExposure(func(step string) {
				feedback.Note(step)
			}); err != nil {
				restrict.Fail(err)
				return err
			}
			restrict.OK(feedback.Val("127.0.0.1"))
			feedback.Note("LAN devices can no longer reach sites, services, or the dashboard")
			feedback.Note("any active remote-setup code has been revoked")
			return nil
		},
	}
}

func newLANShareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "share",
		Short: "Share the current site on a stable LAN port (no DNS setup required on clients)",
		Long: `Assigns a stable port to the current site and starts a host-level reverse
proxy on 0.0.0.0:<port>. Any device on the same network can reach the site
at http://<your-LAN-IP>:<port> without configuring DNS or a resolver.

The proxy rewrites the Host header so nginx routes correctly, and rewrites
absolute URLs in HTML/CSS/JS responses so asset and redirect URLs point to
the LAN address instead of the .test domain.

The assigned port is stored in sites.yaml and reused across restarts.
Run 'lerd lan:unshare' to stop sharing and release the port.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			// A worktree resolves to its parent plus the branch, so running this
			// inside one shares that branch's own domain rather than assigning a
			// port to a site name derived from the checkout directory.
			site, branch, err := ensureSiteAndBranchForCwd()
			if err != nil {
				return err
			}
			siteName := site.Name
			label := siteName
			// Persist the port assignment (daemon will start the proxy).
			port := 0
			action := "lan:share"
			if branch != "" {
				port, err = LANShareEnsureWorktreePort(siteName, branch)
				action += "?branch=" + url.QueryEscape(branch)
				label = branch + "." + site.PrimaryDomain()
			} else {
				port, err = LANShareEnsurePort(siteName)
			}
			if err != nil {
				return err
			}
			// Tell the running daemon to start the proxy now.
			notifyDaemon(site.PrimaryDomain(), action) //nolint:errcheck
			ip, _ := detectPrimaryLANIP()
			if ip == "" {
				ip = "<your-LAN-IP>"
			}
			shareURL := fmt.Sprintf("http://%s:%d", ip, port)
			feedback.Begin()
			feedback.Done("sharing " + label + " at " + feedback.Val(shareURL))
			feedback.Note("other devices on the network can use that URL directly — no DNS setup needed")
			fmt.Println()
			PrintLANShareQR(shareURL)
			fmt.Println()
			feedback.Note("run `lerd lan:unshare` to stop")
			return nil
		},
	}
}

func newLANUnshareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unshare",
		Short: "Stop LAN sharing for the current site",
		RunE: func(_ *cobra.Command, _ []string) error {
			site, branch, err := ensureSiteAndBranchForCwd()
			if err != nil {
				return err
			}
			label := site.Name
			action := "lan:unshare"
			if branch != "" {
				action += "?branch=" + url.QueryEscape(branch)
				label = branch + "." + site.PrimaryDomain()
			}
			// Tell the running daemon to stop the proxy and clear the port.
			// If the daemon is not reachable, clear the port directly so the
			// proxy is not restored on next daemon start.
			if nErr := notifyDaemon(site.PrimaryDomain(), action); nErr != nil {
				if branch != "" {
					_, _, _ = config.RemoveWorktreeLAN(site.Name, branch)
				} else {
					site.LANPort = 0
					_ = config.AddSite(*site)
				}
			}
			feedback.Begin()
			feedback.Done("LAN sharing stopped for " + label)
			return nil
		},
	}
}

// notifyDaemon posts an action to the running lerd-ui daemon API. It is a
// best-effort call; callers should handle errors gracefully.
func notifyDaemon(domain, action string) error {
	url := fmt.Sprintf("http://127.0.0.1:7073/api/sites/%s/%s", domain, action)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// Clear the daemon's cross-origin gate for this trusted local POST.
	req.Header.Set("X-Lerd-CSRF", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func newLANServicesCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "services [on|off|status]",
		Short:     "Control LAN access to managed databases, caches, and services",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"on", "off", "status"},
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.LoadGlobal()
			if err != nil {
				return err
			}

			if args[0] == "status" {
				feedback.Begin()
				switch {
				case cfg.LAN.ServicesExposed && cfg.LAN.Exposed:
					feedback.Done("managed service LAN access is active")
				case cfg.LAN.ServicesExposed:
					feedback.Line("managed service LAN access is enabled but inactive until `lerd lan:expose`")
				default:
					feedback.Line("managed service LAN access is off; services are loopback-only")
				}
				return nil
			}

			enabled := args[0] == "on"
			feedback.Begin()
			update := feedback.Start("updating managed service LAN access")
			if err := SetManagedServiceLANExposure(enabled, nil); err != nil {
				update.Fail(err)
				return err
			}
			if enabled {
				update.OK(feedback.Val("enabled"))
				feedback.Note("development services may use weak or empty credentials; restrict their ports with the host firewall and use only on a trusted network")
				if !cfg.LAN.Exposed {
					feedback.Note("services remain loopback-only until `lerd lan:expose`")
				}
			} else {
				update.OK(feedback.Val("loopback-only"))
			}
			return nil
		},
	}
}

func newLANStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether lerd is currently exposed to the local network",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			feedback.Begin()
			if cfg.LAN.Exposed {
				lanIP, _ := detectPrimaryLANIP()
				if lanIP == "" {
					lanIP = "(unknown)"
				}
				feedback.Done("exposed to the LAN at " + feedback.Val(lanIP))
				if cfg.LAN.ServicesExposed {
					feedback.Note("managed services: exposed")
				} else {
					feedback.Note("managed services: loopback-only")
				}
			} else {
				feedback.Line("loopback-only (127.0.0.1) — LAN devices cannot reach it")
			}
			return nil
		},
	}
}
