package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/desktopnotify"
	"github.com/spf13/cobra"
)

const dashboardURL = "http://lerd.localhost"

// NewDashboardCmd returns the dashboard command.
func NewDashboardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dashboard",
		Short: "Open the Lerd dashboard (the desktop app if installed, else the browser)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			// Prefer the desktop app when it's the registered lerd:// handler;
			// it focuses the running window rather than opening a new tab.
			if desktopnotify.AppInstalled() {
				if err := desktopnotify.OpenApp(""); err == nil {
					fmt.Println("Opening the Lerd desktop app")
					return nil
				}
			}
			fmt.Printf("Opening %s\n", dashboardURL)
			return openBrowser(dashboardURL)
		},
	}
}
