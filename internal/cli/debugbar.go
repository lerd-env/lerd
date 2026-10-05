package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/browsercapture"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewDebugbarCmd returns `lerd debugbar`, which shows or hides the debug bar
// on the pages of the site in this directory.
func NewDebugbarCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "debugbar",
		Short: "Show a debug bar on the pages of the site in this directory",
		Long: `Turn the debug bar on or off for the site in this directory. While it is on,
nginx injects a small bar into the site's HTML pages that sums up the request
behind the page and opens its full detail in place. The setting is kept under
devtools.debugbar in .lerd.yaml when the project has one.`,
	}
	toggle := func(on bool) func(*cobra.Command, []string) error {
		return func(_ *cobra.Command, _ []string) error { return runDebugbarToggle(on) }
	}
	cmd.AddCommand(&cobra.Command{Use: "on", Short: "Show the debug bar", Args: cobra.NoArgs, RunE: toggle(true)})
	cmd.AddCommand(&cobra.Command{Use: "off", Short: "Hide the debug bar", Args: cobra.NoArgs, RunE: toggle(false)})
	cmd.AddCommand(&cobra.Command{Use: "status", Short: "Show whether the debug bar is on", Args: cobra.NoArgs, RunE: runDebugbarStatus})
	return cmd
}

func runDebugbarToggle(on bool) error {
	site, err := siteInCwd()
	if err != nil {
		return err
	}
	res, err := browsercapture.SetDebugbar(*site, on)
	if err != nil {
		return err
	}
	switch {
	case res.NoChange && on:
		fmt.Printf("Debug bar already on for %s.\n", site.Name)
	case res.NoChange:
		fmt.Printf("Debug bar already off for %s.\n", site.Name)
	case on:
		fmt.Printf("Debug bar on for %s. Reload a page to see it.\n", site.Name)
	default:
		fmt.Printf("Debug bar off for %s.\n", site.Name)
	}
	return nil
}

func runDebugbarStatus(_ *cobra.Command, _ []string) error {
	site, err := siteInCwd()
	if err != nil {
		return err
	}
	state := feedback.Amber("off")
	if config.DebugbarFor(*site) {
		state = feedback.Green("on")
	}
	fmt.Printf("Debug bar for %s: %s\n", site.Name, state)
	return nil
}
