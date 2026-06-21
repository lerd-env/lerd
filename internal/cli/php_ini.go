package cli

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// NewPhpIniCmd returns the php:ini command.
func NewPhpIniCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "php:ini [version]",
		Short: "Edit the user php.ini for a PHP version",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runPhpIni,
	}
}

func runPhpIni(_ *cobra.Command, args []string) error {
	version, err := phpExtVersion(args)
	if err != nil {
		return err
	}

	if err := podman.EnsureUserIni(version); err != nil {
		return fmt.Errorf("creating user ini: %w", err)
	}

	path := config.PHPUserIniFile(version)
	launched, err := launchEditor(path)
	if err != nil {
		return err
	}
	if !launched {
		feedback.Begin()
		feedback.Line("user ini file: " + feedback.Val(path))
		feedback.Note("set $EDITOR to open it automatically")
		return nil
	}

	// Ensure the quadlet has the user ini volume mount (may be missing on
	// installations predating the user ini feature).
	if err := podman.WriteFPMQuadlet(version); err != nil {
		return fmt.Errorf("updating quadlet: %w", err)
	}

	short := strings.ReplaceAll(version, ".", "")
	unit := "lerd-php" + short + "-fpm"
	feedback.Begin()
	step := feedback.Start("restarting " + unit)
	if err := podman.RestartUnit(unit); err != nil {
		step.Fail(err)
		return fmt.Errorf("restarting %s: %w", unit, err)
	}
	// Per-site containers (FrankenPHP, custom-FPM) on this version mount the same
	// user ini; restart them so the edit applies there too.
	podman.RestartSiteContainersForVersion(version)
	step.OK("")
	return nil
}
