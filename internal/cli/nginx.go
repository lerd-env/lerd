package cli

import (
	"errors"
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/spf13/cobra"
)

// resolveNginxDomain turns an optional [site] arg plus an optional --branch
// into the domain whose custom nginx override to operate on. No branch means
// the site's primary domain; a branch resolves to that worktree's subdomain
// the same way the daemon does (gitpkg.DetectWorktrees).
func resolveNginxDomain(args []string, branch string) (*config.Site, string, error) {
	name, err := resolveSiteName(args)
	if err != nil {
		return nil, "", err
	}
	site, err := config.FindSite(name)
	if err != nil {
		return nil, "", fmt.Errorf("site %q not found — run 'lerd link' first", name)
	}
	domain, err := siteops.WorktreeDomain(site, branch)
	if err != nil {
		return nil, "", err
	}
	return site, domain, nil
}

// nginxScope maps the --location flag onto the scope. The two override files
// differ only in where the generated vhost includes them, so one boolean beats
// a --scope string with exactly two legal values.
func nginxScope(location bool) siteops.NginxScope {
	if location {
		return siteops.NginxLocationScope
	}
	return siteops.NginxServerScope
}

// addNginxFlags registers the flags every `lerd nginx` subcommand shares.
func addNginxFlags(cmd *cobra.Command, branch *string, location *bool) {
	cmd.Flags().StringVar(branch, "branch", "", "Worktree branch to target instead of the main branch")
	cmd.Flags().BoolVar(location, "location", false, "Target the override included inside the site's request-handling location, where fastcgi_param and proxy_set_header overrides take effect")
}

// NewNginxCmd returns the `lerd nginx` command group for the per-site custom
// nginx override. Saving goes through the same edit service as the web UI, so
// `nginx -t` validation, backups, and the reload all behave identically.
func NewNginxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nginx",
		Short: "Show, edit, or reset a site's custom nginx override, or update nginx",
		Long: "Manage the per-site nginx override included at the end of a site's\n" +
			"server block (custom.d/{domain}.conf). Pass --location for the one\n" +
			"included inside the block that serves the site, the only place a\n" +
			"fastcgi_param or proxy_set_header override takes effect, and\n" +
			"--branch to target a worktree's override instead of the main branch's.",
		// nginx is removed while lerd's own web server serves, so its overrides
		// would do nothing; say where the running server's settings are.
		PersistentPreRunE: func(*cobra.Command, []string) error {
			cfg, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			if kind, _ := cfg.WebServerKind(); kind == config.WebServerGo {
				return errors.New("lerd's own web server is serving, so nginx is not running: use lerd web-server show, edit or reset for its settings, or lerd web-server nginx to switch back")
			}
			return nil
		},
	}
	cmd.AddCommand(newNginxShowCmd(), newNginxEditCmd(), newNginxResetCmd(), newNginxUpdateCmd())
	return cmd
}

func newNginxUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Pull the newest nginx image and restart nginx onto it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			updated, err := podman.UpdateNginx(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			if updated {
				fmt.Fprintln(cmd.OutOrStdout(), "Nginx restarted on the newest image.")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Nginx already runs the newest image.")
			}
			return nil
		},
	}
}

func newNginxShowCmd() *cobra.Command {
	var branch string
	var location bool
	var pathOnly bool
	cmd := &cobra.Command{
		Use:   "show [site]",
		Short: "Print the custom nginx override (or its file path with --path)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, domain, err := resolveNginxDomain(args, branch)
			if err != nil {
				return err
			}
			if pathOnly {
				fmt.Fprintln(cmd.OutOrStdout(), siteops.CustomNginxPath(domain, nginxScope(location)))
				return nil
			}
			got, err := siteops.ReadCustomNginx(domain, nginxScope(location))
			if err != nil {
				return err
			}
			if !got.Exists {
				fmt.Fprintf(cmd.ErrOrStderr(), "No saved override for %s yet; showing the template.\n", domain)
			}
			fmt.Fprint(cmd.OutOrStdout(), got.Body)
			return nil
		},
	}
	addNginxFlags(cmd, &branch, &location)
	cmd.Flags().BoolVar(&pathOnly, "path", false, "Print the override file path and exit")
	return cmd
}

func newNginxEditCmd() *cobra.Command {
	var branch string
	var location bool
	cmd := &cobra.Command{
		Use:   "edit [site]",
		Short: "Open the custom nginx override in $EDITOR, then validate and reload",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, domain, err := resolveNginxDomain(args, branch)
			if err != nil {
				return err
			}
			got, err := siteops.ReadCustomNginx(domain, nginxScope(location))
			if err != nil {
				return err
			}
			// Edit a temp copy so the live file is untouched until SaveCustomNginx
			// snapshots, backs up, validates, and reloads it atomically.
			edited, ok, err := editInEditor(cmd, got.Body, "lerd-nginx-*.conf", siteops.CustomNginxPath(domain, nginxScope(location)))
			if err != nil || !ok {
				return err
			}
			res, err := siteops.SaveCustomNginx(domain, nginxScope(location), edited, got.Exists)
			if err != nil {
				return err
			}
			if !res.OK {
				if res.ValidationOutput != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), res.ValidationOutput)
				}
				return fmt.Errorf("%s", res.Error)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Saved %s and reloaded nginx.\n", domain)
			return nil
		},
	}
	addNginxFlags(cmd, &branch, &location)
	return cmd
}

func newNginxResetCmd() *cobra.Command {
	var branch string
	var location bool
	cmd := &cobra.Command{
		Use:   "reset [site]",
		Short: "Delete the custom nginx override and reload nginx (backups are kept)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, domain, err := resolveNginxDomain(args, branch)
			if err != nil {
				return err
			}
			if err := siteops.ResetCustomNginx(domain, nginxScope(location)); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Reset %s to bundled defaults.\n", domain)
			return nil
		},
	}
	addNginxFlags(cmd, &branch, &location)
	return cmd
}
