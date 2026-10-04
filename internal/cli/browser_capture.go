package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/geodro/lerd/internal/browsercapture"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewBrowserCaptureCmd returns `lerd browser-capture`, which turns the
// reporting of JavaScript errors from site pages on and off.
func NewBrowserCaptureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "browser-capture",
		Short: "Capture JavaScript errors from your sites' pages",
		Long: `Turn browser capture on or off. While it is on, nginx injects a small script
into the HTML of every PHP-FPM, host-proxy and custom-container site that reports uncaught errors, unhandled promise
rejections and, per site, console messages and failed requests to the
dashboard's Debug view and the MCP diag tool. Per-site settings live under
browser_capture in .lerd.yaml.`,
	}
	toggle := func(on bool) func(*cobra.Command, []string) error {
		return func(_ *cobra.Command, _ []string) error { return runBrowserCaptureToggle(on) }
	}
	cmd.AddCommand(&cobra.Command{Use: "on", Short: "Turn browser capture on", Args: cobra.NoArgs, RunE: toggle(true)})
	cmd.AddCommand(&cobra.Command{Use: "off", Short: "Turn browser capture off", Args: cobra.NoArgs, RunE: toggle(false)})
	cmd.AddCommand(&cobra.Command{Use: "status", Short: "Show whether browser capture is on", Args: cobra.NoArgs, RunE: runBrowserCaptureStatus})
	cmd.AddCommand(&cobra.Command{Use: "presets", Short: "List the store's event presets for the site in this directory", Args: cobra.NoArgs, RunE: runBrowserCapturePresets})
	preset := &cobra.Command{Use: "preset", Short: "Add or remove an event preset for the site in this directory"}
	apply := func(add bool) func(*cobra.Command, []string) error {
		return func(_ *cobra.Command, args []string) error { return runBrowserCapturePreset(args[0], add) }
	}
	preset.AddCommand(&cobra.Command{Use: "add <preset>", Short: "Add a preset's events", Args: cobra.ExactArgs(1), RunE: apply(true)})
	preset.AddCommand(&cobra.Command{Use: "remove <preset>", Short: "Remove a preset's events", Args: cobra.ExactArgs(1), RunE: apply(false)})
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

func runBrowserCapturePresets(_ *cobra.Command, _ []string) error {
	site, err := siteInCwd()
	if err != nil {
		return err
	}
	presets := browsercapture.Presets(*site)
	if len(presets) == 0 {
		fmt.Println("The store lists no browser capture presets yet.")
		return nil
	}
	for _, p := range presets {
		var tags []string
		if p.Detected {
			tags = append(tags, feedback.Green("detected"))
		}
		if p.Applied {
			tags = append(tags, "added")
		}
		fmt.Printf("%-10s %-15s %s  %s\n", p.Name, p.Label, strings.Join(p.Contents(), ", "), strings.Join(tags, " "))
	}
	return nil
}

func runBrowserCapturePreset(name string, add bool) error {
	site, err := siteInCwd()
	if err != nil {
		return err
	}
	if err := browsercapture.ApplyPreset(*site, name, add); err != nil {
		return err
	}
	if add {
		fmt.Printf("Added the %s events to %s.\n", name, site.Name)
	} else {
		fmt.Printf("Removed the %s events from %s.\n", name, site.Name)
	}
	return nil
}

func runBrowserCaptureToggle(on bool) error {
	res, err := browsercapture.SetEnabled(on)
	if err != nil {
		return err
	}
	switch {
	case res.NoChange && on:
		fmt.Println("Browser capture already on.")
	case res.NoChange:
		fmt.Println("Browser capture already off.")
	case on:
		fmt.Println("Browser capture on. JavaScript errors from your sites now show in the Debug view.")
	default:
		fmt.Println("Browser capture off.")
	}
	return nil
}

func runBrowserCaptureStatus(_ *cobra.Command, _ []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	state := feedback.Amber("off")
	if cfg.IsBrowserCaptureEnabled() {
		state = feedback.Green("on")
	}
	fmt.Printf("Browser capture: %s\n", state)
	site, err := siteInCwd()
	if err != nil {
		return nil
	}
	s := config.BrowserCaptureFor(*site)
	none := func(v []string) string {
		if len(v) == 0 {
			return "none"
		}
		return strings.Join(v, ", ")
	}
	fmt.Printf("Site %s:  enabled %v, console %s, network %s\n", site.Name, s.Enabled, none(s.Console), none(s.Network))
	return nil
}
