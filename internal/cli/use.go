package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewUseCmd returns the use command.
func NewUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <version>",
		Short: "Set the global PHP version",
		Args:  cobra.ExactArgs(1),
		RunE:  runUse,
	}
}

func runUse(_ *cobra.Command, args []string) error {
	version := args[0]

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	cfg.PHP.DefaultVersion = version
	if err := config.SaveGlobal(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	feedback.Begin()
	feedback.Done("default PHP set to " + feedback.Val(version))

	// Ensure FPM quadlet exists for this version
	if err := ensureFPMQuadlet(version); err != nil {
		feedback.Warn("FPM quadlet for PHP %s: %v", version, err)
	}

	return nil
}
