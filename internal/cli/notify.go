package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewNotifyCmd returns the parent `lerd notify` command. Subcommands flip
// the global notification toggle (dashboard banners + Web Push fanout) and
// report current status.
func NewNotifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Globally enable or disable lerd notifications",
		Long: `Globally toggle the lerd notifier. When off, neither in-dashboard
banners nor Web Push are dispatched, regardless of per-device prefs.
On by default; toggle via ` + "`lerd notify on`" + ` and ` + "`lerd notify off`" + `.`,
	}
	cmd.AddCommand(newNotifyOnCmd())
	cmd.AddCommand(newNotifyOffCmd())
	cmd.AddCommand(newNotifyStatusCmd())
	return cmd
}

func newNotifyOnCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "on",
		Short: "Enable notifications globally",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runNotifyToggle(true)
		},
	}
}

func newNotifyOffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "off",
		Short: "Disable notifications globally",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runNotifyToggle(false)
		},
	}
}

func newNotifyStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether notifications are globally enabled",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runNotifyStatus()
		},
	}
}

func runNotifyToggle(enable bool) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if cfg.IsNotificationsEnabled() == enable {
		fmt.Printf("Notifications already %s.\n", notifyStateWord(enable))
		return nil
	}
	cfg.SetNotificationsEnabled(enable)
	if err := config.SaveGlobal(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Notifications %s.\n", notifyStateWord(enable))
	return nil
}

func runNotifyStatus() error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	state := feedback.Amber("disabled")
	if cfg.IsNotificationsEnabled() {
		state = feedback.Green("enabled")
	}
	fmt.Printf("Notifications: %s\n", state)
	return nil
}

func notifyStateWord(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}
