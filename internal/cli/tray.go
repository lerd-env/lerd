package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/tray"
	"github.com/spf13/cobra"
)

// NewTrayCmd returns the tray command. Run bare it launches the applet; its
// `icon` subcommand chooses the running-icon style.
func NewTrayCmd() *cobra.Command {
	var mono bool
	cmd := &cobra.Command{
		Use:   "tray",
		Short: "Launch the system tray applet",
		RunE: func(_ *cobra.Command, _ []string) error {
			return tray.Run(mono)
		},
	}
	cmd.Flags().BoolVar(&mono, "mono", false, "Use a monochrome template icon (OS recolors it); default is the colour icon that flips white/red with lerd state")
	cmd.AddCommand(newTrayIconCmd())
	return cmd
}

// newTrayIconCmd chooses between the theme-adaptive running icon and the
// always-visible high-contrast one, for panels where light/dark detection
// guesses wrong (mixed themes like KDE Breeze Twilight).
func newTrayIconCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "icon [default|high-contrast]",
		Short: "Choose the tray running-icon style",
		Long: `Choose how the running tray icon is drawn.

default        theme-adaptive, white on dark panels and dark on light ones
high-contrast  a single green icon that stays visible on any panel, including
               mixed themes like KDE Breeze Twilight where detection guesses wrong

Run with no argument to print the current style.`,
		ValidArgs: []string{"default", "high-contrast"},
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runTrayIconStatus()
			}
			return runTrayIconSet(args[0] == "high-contrast")
		},
	}
}

func runTrayIconSet(highContrast bool) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if cfg.IsHighContrastTrayIcon() == highContrast {
		fmt.Printf("Tray icon already set to %s.\n", trayIconStyleWord(highContrast))
		return nil
	}
	cfg.SetHighContrastTrayIcon(highContrast)
	if err := config.SaveGlobal(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Tray icon set to %s.\n", trayIconStyleWord(highContrast))
	return nil
}

func runTrayIconStatus() error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	fmt.Printf("Tray icon: %s\n", feedback.Green(trayIconStyleWord(cfg.IsHighContrastTrayIcon())))
	return nil
}

func trayIconStyleWord(highContrast bool) string {
	if highContrast {
		return "high-contrast"
	}
	return "default"
}
