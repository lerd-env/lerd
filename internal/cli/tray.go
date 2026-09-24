package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/services"
	lerdSystemd "github.com/geodro/lerd/internal/systemd"
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
	cmd.AddCommand(newTrayIconCmd(), newTrayOnCmd(), newTrayOffCmd())
	return cmd
}

// newTrayOnCmd puts the tray back, for the desktop where it was turned off.
func newTrayOnCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "on",
		Short: "Run the system tray applet with lerd",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return runTraySet(true) },
	}
}

// newTrayOffCmd stops the applet and keeps `lerd start` from bringing it back,
// for desktops that already show the same state somewhere else.
func newTrayOffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "off",
		Short: "Stop running the system tray applet",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return runTraySet(false) },
	}
}

func runTraySet(enabled bool) error {
	changed, err := ApplyTray(enabled)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Printf("System tray already %s.\n", trayStateWord(enabled))
		return nil
	}
	fmt.Printf("System tray %s.\n", trayStateWord(enabled))
	return nil
}

// ApplyTray persists the tray preference and brings the running tray in line
// with it, so the CLI, the tray menu and the dashboard all take one path. It
// reports whether the preference actually changed, which only the CLI prints.
func ApplyTray(enabled bool) (bool, error) {
	changed, err := setTrayPreference(enabled)
	if err != nil {
		return false, err
	}
	if !enabled {
		killTray()
		disableTrayUnit()
		return changed, nil
	}
	// Re-enabling only makes sense where the unit would have been enabled in the
	// first place: autostart on, and a helper that can actually run.
	if lerdSystemd.IsAutostartEnabled() && tray.Unavailable(tray.HelperPath()) == "" {
		_ = services.Mgr.Enable("lerd-tray")
	}
	return changed, launchTray()
}

// setTrayPreference persists the preference alone, reporting whether it moved.
func setTrayPreference(enabled bool) (bool, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return false, fmt.Errorf("loading config: %w", err)
	}
	if cfg.IsTrayEnabled() == enabled {
		return false, nil
	}
	cfg.SetTrayEnabled(enabled)
	if err := config.SaveGlobal(cfg); err != nil {
		return false, fmt.Errorf("saving config: %w", err)
	}
	return true, nil
}

// healArmedTrayUnit disarms a tray unit left enabled while the preference is
// off, reporting whether it had to. The desktop session starts an armed unit at
// login without consulting the preference, so the two coming apart, which a
// restored config or an older build can do, brings the tray back on every boot
// with nothing in the settings to explain it.
func healArmedTrayUnit() bool {
	if trayEnabled() || !services.Mgr.IsEnabled("lerd-tray") {
		return false
	}
	disableTrayUnit()
	return true
}

// trayEnabled reports the configured preference for the start and install
// paths. A config that will not load keeps the tray, so a broken file never
// silently removes a surface the user expects.
func trayEnabled() bool {
	cfg, err := config.LoadGlobal()
	return err != nil || cfg.IsTrayEnabled()
}

func trayStateWord(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
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
