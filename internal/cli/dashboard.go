package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/dashboard"
	"github.com/geodro/lerd/internal/desktopapp"
	"github.com/geodro/lerd/internal/desktopnotify"
	"github.com/spf13/cobra"
)

// NewDashboardCmd returns the dashboard command.
func NewDashboardCmd() *cobra.Command {
	var withSplash bool
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Open the Lerd dashboard, starting Lerd first if it is not running",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDashboard(withSplash)
		},
	}
	// The desktop entry passes this: clicked from an application launcher there
	// is no terminal for the start's output, and a cold start takes the better
	// part of a minute.
	cmd.Flags().BoolVar(&withSplash, "splash", false,
		"Show a progress window while Lerd starts (used by the desktop entry)")
	return cmd
}

func runDashboard(withSplash bool) error {
	// Opening the dashboard on a stopped lerd is a request to work on
	// something, so bring the stack up rather than land on a page whose
	// only content is a button to press.
	if !dashboard.Serving() {
		if err := startForDashboard(withSplash); err != nil {
			return err
		}
	}
	// Prefer the desktop app when it's the registered lerd:// handler;
	// it focuses the running window rather than opening a new tab.
	if desktopnotify.AppInstalled() {
		if err := desktopnotify.OpenApp(""); err == nil {
			fmt.Println("Opening the Lerd desktop app")
			return nil
		}
	}
	url := dashboard.URL()
	fmt.Printf("Opening %s\n", url)
	return openDashboard(url)
}

// startForDashboard runs the start, drawing a progress window when the caller
// has no terminal to read. The events are the ones the dashboard's own start
// stream carries, so the window counts units the same way the page does.
func startForDashboard(withSplash bool) error {
	if !withSplash {
		return startLerd(nil, nil)
	}
	window := desktopnotify.StartProgress("Starting "+desktopapp.Name, "Preparing the environment")
	defer window.Close()

	total, done := 0, 0
	return startLerd(func(e StartEvent) {
		switch {
		case e.Total > 0:
			total = e.Total
		case e.Phase == "unit":
			done++
			if e.Unit != "" {
				window.Step(e.Unit)
			}
			window.Percent(done, total)
		case e.Phase == "step" && e.Step != "":
			window.Step(e.Step)
		}
	}, nil)
}
