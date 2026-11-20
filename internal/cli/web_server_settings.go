package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/cfgedit"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/geodro/lerd/internal/webcustom"
)

// webSettings is the settings file one command acts on: a site's (or a
// worktree's) or, with --global, the one applied to every site.
type webSettings struct {
	label string
	path  string
	read  func() (cfgedit.Content, error)
	save  func(string, bool) (cfgedit.SaveResult, error)
	reset func() error
}

func resolveWebSettings(args []string, branch string, global bool) (webSettings, error) {
	if global {
		return webSettings{
			label: "every site",
			path:  webcustom.GlobalPath(),
			read:  siteops.ReadGlobalWeb,
			save:  siteops.SaveGlobalWeb,
			reset: siteops.ResetGlobalWeb,
		}, nil
	}
	_, domain, err := resolveNginxDomain(args, branch)
	if err != nil {
		return webSettings{}, err
	}
	scope := siteops.NginxWebScope
	return webSettings{
		label: domain,
		path:  siteops.CustomNginxPath(domain, scope),
		read:  func() (cfgedit.Content, error) { return siteops.ReadCustomNginx(domain, scope) },
		save: func(body string, backup bool) (cfgedit.SaveResult, error) {
			return siteops.SaveCustomNginx(domain, scope, body, backup)
		},
		reset: func() error { return siteops.ResetCustomNginx(domain, scope) },
	}, nil
}

func addWebSettingsFlags(cmd *cobra.Command, branch *string, global *bool) {
	cmd.Flags().StringVar(branch, "branch", "", "Worktree branch to target instead of the main branch")
	cmd.Flags().BoolVar(global, "global", false, "Target the settings applied to every site")
}

func newWebServerShowCmd() *cobra.Command {
	var branch string
	var global, pathOnly bool
	cmd := &cobra.Command{
		Use:   "show [site]",
		Short: "Print a site's settings for lerd's own web server (or its path with --path)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := resolveWebSettings(args, branch, global)
			if err != nil {
				return err
			}
			if pathOnly {
				fmt.Fprintln(cmd.OutOrStdout(), ws.path)
				return nil
			}
			got, err := ws.read()
			if err != nil {
				return err
			}
			if !got.Exists {
				fmt.Fprintf(cmd.ErrOrStderr(), "No saved settings for %s yet; showing the template.\n", ws.label)
			}
			fmt.Fprint(cmd.OutOrStdout(), got.Body)
			return nil
		},
	}
	addWebSettingsFlags(cmd, &branch, &global)
	cmd.Flags().BoolVar(&pathOnly, "path", false, "Print the settings file path and exit")
	return cmd
}

func newWebServerEditCmd() *cobra.Command {
	var branch string
	var global bool
	cmd := &cobra.Command{
		Use:   "edit [site]",
		Short: "Open a site's settings for lerd's own web server in $EDITOR, then validate and save",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := resolveWebSettings(args, branch, global)
			if err != nil {
				return err
			}
			got, err := ws.read()
			if err != nil {
				return err
			}
			edited, ok, err := editInEditor(cmd, got.Body, "lerd-web-*.yaml", ws.path)
			if err != nil || !ok {
				return err
			}
			res, err := ws.save(edited, got.Exists)
			if err != nil {
				return err
			}
			if !res.OK {
				if res.ValidationOutput != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), res.ValidationOutput)
				}
				return fmt.Errorf("%s", res.Error)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Saved the settings for %s; they apply from the next request.\n", ws.label)
			return nil
		},
	}
	addWebSettingsFlags(cmd, &branch, &global)
	return cmd
}

func newWebServerResetCmd() *cobra.Command {
	var branch string
	var global bool
	cmd := &cobra.Command{
		Use:   "reset [site]",
		Short: "Delete a site's settings for lerd's own web server (backups are kept)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := resolveWebSettings(args, branch, global)
			if err != nil {
				return err
			}
			if err := ws.reset(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Reset the settings for %s.\n", ws.label)
			return nil
		},
	}
	addWebSettingsFlags(cmd, &branch, &global)
	return cmd
}
