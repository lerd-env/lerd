package cli

import (
	"fmt"
	"os"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewPathDisableCmd returns the path:disable command, which takes lerd's shims
// (php, composer, node, npm, npx…) off the shell PATH for users who prefer
// typing `lerd php` explicitly — e.g. so a host PHP's exec() keeps the real
// host environment. The choice is persisted, so installs and updates stop
// re-writing the rc entry.
func NewPathDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path:disable",
		Short: "Take lerd's shims (php, composer, node…) off your shell PATH",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := applyPathShim(true); err != nil {
				return err
			}
			feedback.Begin()
			feedback.Done("PATH shim disabled — open a new shell for it to take effect")
			feedback.Note("`lerd php`, `lerd composer`, `lerd npm` keep working as before")
			feedback.Note("re-enable with `lerd path:enable`")
			return nil
		},
	}
}

// NewPathEnableCmd returns the path:enable command, the inverse of path:disable.
func NewPathEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path:enable",
		Short: "Put lerd's shims (php, composer, node…) back on your shell PATH",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := applyPathShim(false); err != nil {
				return err
			}
			feedback.Begin()
			feedback.Done("PATH shim enabled — open a new shell for it to take effect")
			return nil
		},
	}
}

// applyPathShim persists the PATH-shim choice and applies it to the shell rc
// files right away: disabling removes the entry everywhere, enabling writes it
// for the current shell.
func applyPathShim(disabled bool) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	cfg.Shims.PathDisabled = disabled
	if err := config.SaveGlobal(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if disabled {
		removeShellPathEntry(home)
		return nil
	}
	return writeShellPathEntry(home, config.BinDir())
}
