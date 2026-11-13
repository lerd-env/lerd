package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/plugins"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/spf13/cobra"
)

// NewPluginCmd is `lerd plugin`, which installs and lists user plugins.
func NewPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Install, create and list lerd plugins",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List installed plugins and any that failed to load",
		Args:  cobra.NoArgs,
		RunE:  runPluginList,
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "new <name>",
		Short: "Create a plugin from the boilerplate, ready to edit",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dest := filepath.Join(plugins.Dir(), args[0])
			if err := plugins.Scaffold(args[0], dest); err != nil {
				return err
			}
			feedback.Done("Created plugin " + args[0] + " in " + dest)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "install <name|path|git-url>",
		Short: "Install a plugin from the catalog at its approved commit, or from a folder or git repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			install := plugins.Install
			if plugins.IsCatalogName(args[0]) {
				install = plugins.InstallListed
			}
			p, err := install(args[0])
			if err != nil {
				return err
			}
			feedback.Done("Installed plugin " + p.Name)
			return askPluginNotifications(p.Name)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "search [term]",
		Short: "List the approved plugins in the catalog",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runPluginSearch,
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "update [name...]",
		Short: "Move catalog plugins to their newly approved commit (all of them when none is named)",
		RunE:  runPluginUpdate,
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed plugin, keeping the data it stored",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := serviceops.RemovePlugin(args[0]); err != nil {
				return err
			}
			feedback.Done("Removed plugin " + args[0] + " (its data stays in " + plugins.DataDirFor(args[0]) + ")")
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "keep <name>",
		Short: "Keep using a plugin the catalog revoked, after reading why",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := plugins.KeepRevoked(args[0]); err != nil {
				return err
			}
			feedback.Done("Kept plugin " + args[0] + "; it runs again until the catalog revokes it again")
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:       "notifications <name> on|off",
		Short:     "Allow or stop a plugin's notifications",
		Args:      cobra.ExactArgs(2),
		ValidArgs: []string{"on", "off"},
		RunE: func(_ *cobra.Command, args []string) error {
			if args[1] != "on" && args[1] != "off" {
				return fmt.Errorf("expected on or off, got %q", args[1])
			}
			if err := plugins.SetNotifications(args[0], args[1] == "on"); err != nil {
				return err
			}
			feedback.Done(fmt.Sprintf("Notifications from %s %s", args[0], args[1]))
			return nil
		},
	})
	cmd.AddCommand(siteSwitchCommand("page", "Switch a plugin's page script on or off for the site in this directory", "page script", "page_script", siteops.SetPagePlugin))
	cmd.AddCommand(siteSwitchCommand("card", "Switch a plugin's card on or off on the Overview of the site in this directory", "overview card", "overview_card", siteops.SetOverviewCard))
	cmd.AddCommand(siteSwitchCommand("tab", "Show or hide a plugin's tab on the site in this directory", "site tab", "site_tab", siteops.SetSiteTab))
	return cmd
}

func runPluginList(_ *cobra.Command, _ []string) error {
	list, errs := plugins.Load()
	if len(list) == 0 && len(errs) == 0 {
		feedback.Note("No plugins installed. Create one with `lerd plugin new <name>`.")
		return nil
	}
	rows := make([][]string, 0, len(list)+len(errs))
	for _, p := range list {
		rows = append(rows, []string{p.Name, p.Version, pluginOrigin(p.Name), p.Description})
	}
	for _, e := range errs {
		rows = append(rows, []string{filepath.Base(e.Dir), feedback.Red("broken"), "", e.Error()})
	}
	feedback.Table([]string{"Plugin", "Version", "Source", "Description"}, rows)
	return nil
}

// pluginOrigin says where an installed plugin came from: the catalog commit it
// is at, flagged when a newer one was approved, or that it is not listed.
func pluginOrigin(name string) string {
	src, ok := plugins.InstalledSource(name)
	if !ok {
		return feedback.Dim("not listed")
	}
	origin := "catalog " + src.Commit[:7]
	if _, newer := plugins.PendingUpdate(name); newer {
		origin += " " + feedback.Amber("update")
	}
	return origin
}

func runPluginSearch(_ *cobra.Command, args []string) error {
	c, err := plugins.LoadCatalog()
	if err != nil {
		return err
	}
	term := ""
	if len(args) == 1 {
		term = strings.ToLower(args[0])
	}
	var rows [][]string
	for _, l := range c.Plugins {
		hay := strings.ToLower(strings.Join(append([]string{l.Name, l.Description, l.Author}, l.Tags...), " "))
		if term != "" && !strings.Contains(hay, term) {
			continue
		}
		rows = append(rows, []string{l.Name, l.Author, l.Description})
	}
	if len(rows) == 0 {
		feedback.Note("No plugin in the catalog matches that.")
		return nil
	}
	feedback.Table([]string{"Plugin", "Author", "Description"}, rows)
	feedback.Note("Install one with `lerd plugin install <name>`.")
	return nil
}

func runPluginUpdate(_ *cobra.Command, args []string) error {
	names := args
	if len(names) == 0 {
		list, _ := plugins.Load()
		for _, p := range list {
			if _, ok := plugins.PendingUpdate(p.Name); ok {
				names = append(names, p.Name)
			}
		}
		if len(names) == 0 {
			feedback.Note("Every catalog plugin is at its approved commit.")
			return nil
		}
	}
	for _, name := range names {
		if err := plugins.UpdateListed(name); err != nil {
			return err
		}
		feedback.Done("Updated plugin " + name)
		if err := askPluginNotifications(name); err != nil {
			return err
		}
	}
	return nil
}

// AddPluginCommands registers `<plugin>:<command>` for every installed plugin.
// A name lerd already uses keeps lerd's meaning; broken plugins are skipped
// here and reported by `lerd plugin list`.
func AddPluginCommands(root *cobra.Command) {
	list, _ := plugins.Load()
	for _, p := range list {
		for _, c := range p.Commands {
			name := p.Name + ":" + c.Name
			if commandExists(root, name) {
				continue
			}
			root.AddCommand(newPluginRunCmd(p, c, name))
		}
	}
}

func newPluginRunCmd(p *plugins.Plugin, c plugins.Command, name string) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              c.Usage,
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			run, err := p.Command(c.Run, plugins.SurfaceCLI, args, cwdSiteEnv())
			if err != nil {
				return err
			}
			var stderr plugins.Tail
			run.Stdin, run.Stdout, run.Stderr = os.Stdin, os.Stdout, io.MultiWriter(os.Stderr, &stderr)
			err = run.Run()
			if exit, ok := err.(*exec.ExitError); ok {
				if !c.Expects(exit.ExitCode()) {
					offerPluginReport(plugins.Failure{Plugin: p.Name, Surface: plugins.SurfaceCLI, What: c.Name, Error: strings.TrimSpace(stderr.String() + "\n" + err.Error())})
				}
				os.Exit(exit.ExitCode())
			}
			return err
		},
	}
}

// offerPluginReport records a failed plugin command and, when the plugin came
// from the catalog, prints where to report it. The user watched it fail, so no
// notification follows.
func offerPluginReport(f plugins.Failure) {
	plugins.Record(f)
	if u := f.ReportURL(); u != "" {
		fmt.Fprintf(os.Stderr, "\nPlugin %s failed. Report it to its author: %s\n", f.Plugin, u)
	}
}

// postPluginFailure asks the dashboard to raise the notification for a
// failure recorded in this process, such as a hook the watcher ran.
func postPluginFailure(f plugins.Failure) {
	body, _ := json.Marshal(map[string]string{"plugin": f.Plugin})
	_, _, _ = postUnix("/api/internal/plugin-failed", body)
}

func init() { plugins.SetFailureNotifier(postPluginFailure) }

// cwdSiteEnv tells a plugin which site the user is standing in, if any.
func cwdSiteEnv() map[string]string {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	site, err := config.FindSiteByPath(cwd)
	if err != nil {
		return nil
	}
	return plugins.Env(plugins.SitePayload(site))
}

func commandExists(root *cobra.Command, name string) bool {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return true
		}
	}
	return false
}

// siteSwitchCommand is `lerd plugin <use> <name> on|off`, switching one thing a
// plugin puts on a site for the site linked in the working directory.
func siteSwitchCommand(use, short, what, kind string, set func(*config.Site, string, bool) error) *cobra.Command {
	return &cobra.Command{
		Use:       use + " <name> on|off",
		Short:     short,
		Args:      cobra.ExactArgs(2),
		ValidArgs: []string{"on", "off"},
		RunE: func(_ *cobra.Command, args []string) error {
			if args[1] != "on" && args[1] != "off" {
				return fmt.Errorf("expected on or off, got %q", args[1])
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			site, err := config.FindSiteByPath(cwd)
			if err != nil {
				return fmt.Errorf("no lerd site here; run this from a linked project")
			}
			if err := switchSitePlugin(site, args[0], kind, args[1] == "on", set); err != nil {
				return err
			}
			feedback.Done(fmt.Sprintf("%s %s %s for %s", args[0], what, args[1], site.Name))
			return nil
		},
	}
}

// askPluginNotifications asks once whether a plugin may send notifications; an
// earlier answer holds. Without a terminal nothing is assumed, it stays pending
// and the dashboard asks next.
func askPluginNotifications(name string) error {
	p, err := plugins.Find(name)
	if err != nil || plugins.NotificationState(p) != plugins.NotificationsPending {
		return nil
	}
	if !isInteractive() {
		feedback.Note(fmt.Sprintf("%s asks to send you notifications. Allow it with `lerd plugin notifications %s on`.", name, name))
		return nil
	}
	allowed := feedback.Confirm(fmt.Sprintf("Allow %s to send you notifications?", name), false)
	if err := plugins.SetNotifications(name, allowed); err != nil {
		return err
	}
	if !allowed {
		feedback.Note(fmt.Sprintf("Change it any time with `lerd plugin notifications %s on`.", name))
	}
	return nil
}

// uiSocketPath is where a running lerd-ui listens; swapped out in tests.
var uiSocketPath = config.UISocketPath

// switchSitePlugin hands the switch to a running lerd-ui, which tells every
// open dashboard, and makes it here with set only when none answers.
func switchSitePlugin(site *config.Site, plugin, kind string, on bool, set func(*config.Site, string, bool) error) error {
	body, err := json.Marshal(map[string]any{"site": site.Name, "plugin": plugin, "kind": kind, "on": on})
	if err != nil {
		return err
	}
	client := &http.Client{
		// A page script waits for nginx to reload before lerd-ui answers.
		Timeout: 30 * time.Second,
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", uiSocketPath())
		}},
	}
	req, err := http.NewRequest(http.MethodPost, "http://lerd-ui/api/page-plugins", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lerd-CSRF", "1")
	resp, err := client.Do(req)
	if err != nil {
		return set(site, plugin, on)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return errors.New(strings.TrimSpace(string(msg)))
	}
	return nil
}
