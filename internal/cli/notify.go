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
	cmd.AddCommand(newNotifyTargetCmd())
	cmd.AddCommand(newNotifyStatusCmd())
	return cmd
}

func newNotifyTargetCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "target <browser|native>",
		Short:     "Choose the notification delivery sink",
		Long:      "Route notifications to the browser (WebSocket + Web Push) or to native desktop notifications posted by the daemon. Native is Linux only for now.",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{config.NotifyTargetBrowser, config.NotifyTargetNative},
		RunE: func(_ *cobra.Command, args []string) error {
			return runNotifyTarget(args[0])
		},
	}
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

func runNotifyTarget(target string) error {
	if target != config.NotifyTargetBrowser && target != config.NotifyTargetNative {
		return fmt.Errorf("target must be %q or %q", config.NotifyTargetBrowser, config.NotifyTargetNative)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if cfg.NotificationTarget() == target {
		fmt.Printf("Notification target already %s.\n", target)
		return nil
	}
	cfg.SetNotificationTarget(target)
	if err := config.SaveGlobal(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Notification target set to %s.\n", target)
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
	fmt.Printf("Delivery: %s\n", cfg.NotificationTarget())
	return nil
}

func notifyStateWord(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}
