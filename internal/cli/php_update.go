package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/tools"
)

// NewPhpUpdateCmd returns the php:update command.
func NewPhpUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "php:update [version]",
		Short: "Update PHP to the newest published patch",
		Long: `Update PHP to the newest patch lerd publishes.

On the native runtime this downloads the new build and restarts the pools that
run it. On the container runtime it rebuilds the FPM images from the newest
base, which is what 'lerd php:rebuild' does.

Pass a version (e.g. 8.4) to update only that one, or omit it to update every
installed version.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			if cfg.PHPRuntimeMode() != config.PHPRuntimeNative {
				return runPhpRebuild(cmd, args)
			}
			return runNativePHPUpdate(cmd, args)
		},
	}
}

// runNativePHPUpdate brings the installed native builds up to the published
// patch. The pins are refreshed past their cache first: this is the command a
// person runs precisely because they want to know about a build published in
// the last day, and the passive path only looks once every 24h.
func runNativePHPUpdate(cmd *cobra.Command, args []string) error {
	versions, err := nativeVersionsToUpdate(args)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		feedback.Line("no native PHP versions installed")
		return nil
	}
	w := cmd.OutOrStdout()
	feedback.Begin()
	pins := &pinnedTools{m: tools.Refresh(context.Background())}

	updated := 0
	for _, v := range versions {
		patch, need := nativeUpdatePlan(pins.m.Tools[nativeTool(v)].Version, tools.InstalledVersion(nativeTool(v)))
		if patch == "" {
			feedback.Warn("lerd publishes no native build for php %s", v)
			continue
		}
		if !need {
			feedback.Line("php " + v + " is already on " + patch)
			continue
		}
		if _, err := installNativePHP(pins, v, w); err != nil {
			return err
		}
		// The running pool holds the old binary and its extensions open, so it
		// keeps serving them until it is replaced.
		if err := nativephp.Reload(v); err != nil {
			feedback.Warn("restarting native php-fpm %s: %v", v, err)
		}
		feedback.Line("php " + v + " updated to " + patch)
		updated++
	}
	if updated == 0 {
		feedback.Done("PHP is up to date")
		return nil
	}
	feedback.Done(fmt.Sprintf("updated %d PHP version(s)", updated))
	return nil
}

// nativeVersionsToUpdate resolves the command's argument to the versions to act
// on: the one named, or every native build on disk.
func nativeVersionsToUpdate(args []string) ([]string, error) {
	if len(args) == 1 {
		v, err := config.NormalizePHPVersion(args[0])
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(nativephp.BinaryPath(v)); err != nil {
			return nil, fmt.Errorf("php %s has no native build installed; add it with 'lerd use %s'", v, v)
		}
		return []string{v}, nil
	}
	return nativephp.ListInstalled(), nil
}
