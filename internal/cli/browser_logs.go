package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/geodro/lerd/internal/browserlogs"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewBrowserLogsCmd returns `lerd browser-logs`, which turns the
// reporting of JavaScript errors from site pages on and off.
func NewBrowserLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "browser-logs",
		Short: "Capture JavaScript errors from your sites' pages",
		Long: `Turn browser logs on or off for a site, the one in this directory unless
one is named. While debug capture is on (lerd dump on), nginx injects a small
script into the HTML of every site that opted in, which reports uncaught errors,
unhandled promise rejections and, per site, console messages and failed
requests to the dashboard's Debug view and the MCP diag tool. Settings are
kept in lerd's site registry, never in the project.`,
	}
	toggle := func(on bool) func(*cobra.Command, []string) error {
		return func(_ *cobra.Command, args []string) error { return runBrowserLogsToggle(args, on) }
	}
	cmd.AddCommand(&cobra.Command{Use: "on [site]", Short: "Turn browser logs on for a site", Args: cobra.MaximumNArgs(1), RunE: toggle(true)})
	cmd.AddCommand(&cobra.Command{Use: "off [site]", Short: "Turn browser logs off for a site", Args: cobra.MaximumNArgs(1), RunE: toggle(false)})
	cmd.AddCommand(&cobra.Command{Use: "status", Short: "Show whether browser logs are on for the site in this directory", Args: cobra.NoArgs, RunE: runBrowserLogsStatus})
	cmd.AddCommand(&cobra.Command{Use: "presets", Short: "List the store's event presets for the site in this directory", Args: cobra.NoArgs, RunE: runBrowserLogsPresets})
	preset := &cobra.Command{Use: "preset", Short: "Switch an event preset on or off for the site in this directory"}
	apply := func(add bool) func(*cobra.Command, []string) error {
		return func(_ *cobra.Command, args []string) error { return runBrowserLogsPreset(args[0], add) }
	}
	preset.AddCommand(&cobra.Command{Use: "on <preset>", Short: "Report a preset's events", Args: cobra.ExactArgs(1), RunE: apply(true)})
	preset.AddCommand(&cobra.Command{Use: "off <preset>", Short: "Stop reporting a preset's events", Args: cobra.ExactArgs(1), RunE: apply(false)})
	cmd.AddCommand(preset)
	return cmd
}

func siteInCwd() (*config.Site, error) {
	cwd, _ := os.Getwd()
	site, err := config.FindSiteByPath(cwd)
	if err != nil || site == nil {
		return nil, fmt.Errorf("no linked site in %s", cwd)
	}
	return site, nil
}

func runBrowserLogsPresets(_ *cobra.Command, _ []string) error {
	site, err := siteInCwd()
	if err != nil {
		return err
	}
	presets := browserlogs.Presets(*site)
	if len(presets) == 0 {
		fmt.Println("The store lists no browser logs presets yet.")
		return nil
	}
	for _, p := range presets {
		var tags []string
		if p.Detected {
			tags = append(tags, feedback.Green("detected"))
		}
		if p.Active {
			tags = append(tags, "on")
		}
		fmt.Printf("%-10s %-15s %s  %s\n", p.Name, p.Label, strings.Join(p.Contents(), ", "), strings.Join(tags, " "))
	}
	return nil
}

func runBrowserLogsPreset(name string, add bool) error {
	site, err := siteInCwd()
	if err != nil {
		return err
	}
	if err := browserlogs.SetPreset(*site, name, add); err != nil {
		return err
	}
	if add {
		fmt.Printf("Preset %s on for %s.\n", name, site.Name)
	} else {
		fmt.Printf("Preset %s off for %s.\n", name, site.Name)
	}
	return nil
}

// siteFromArgs is the site named in args, or the one in this directory.
func siteFromArgs(args []string) (*config.Site, error) {
	if len(args) == 0 {
		return siteInCwd()
	}
	site, err := config.FindSiteByRef(args[0])
	if err != nil || site == nil {
		return nil, fmt.Errorf("no linked site %q", args[0])
	}
	return site, nil
}

func runBrowserLogsToggle(args []string, on bool) error {
	site, err := siteFromArgs(args)
	if err != nil {
		return err
	}
	res, err := browserlogs.SetSite(*site, on)
	if err != nil {
		return err
	}
	switch {
	case res.NoChange && on:
		fmt.Printf("Browser logs already on for %s.\n", site.Name)
	case res.NoChange:
		fmt.Printf("Browser logs already off for %s.\n", site.Name)
	case on:
		fmt.Printf("Browser logs on for %s.\n", site.Name)
	default:
		fmt.Printf("Browser logs off for %s.\n", site.Name)
	}
	if cfg, err := config.LoadGlobal(); on && err == nil && !cfg.IsDumpsEnabled() {
		fmt.Println("Debug capture is off, so its pages get the script once you run `lerd dump on`.")
	}
	return nil
}

func runBrowserLogsStatus(_ *cobra.Command, _ []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	state := feedback.Amber("off")
	if cfg.IsDumpsEnabled() {
		state = feedback.Green("on")
	}
	fmt.Printf("Debug capture: %s\n", state)
	site, err := siteInCwd()
	if err != nil {
		return nil
	}
	s := config.BrowserLogsFor(*site)
	none := func(v []string) string {
		if len(v) == 0 {
			return "none"
		}
		return strings.Join(v, ", ")
	}
	fmt.Printf("Site %s:  enabled %v, console %s, network %s\n", site.Name, s.Enabled, none(s.Console), none(s.Network))
	return nil
}
