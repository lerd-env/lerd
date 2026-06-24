package cli

import (
	"fmt"
	"os"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/xdebugops"
	"github.com/spf13/cobra"
)

// NewXdebugCmd returns the xdebug parent command with on/off/status subcommands.
func NewXdebugCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "xdebug",
		Short: "Toggle Xdebug for a PHP version",
	}
	cmd.AddCommand(newXdebugOnCmd())
	cmd.AddCommand(newXdebugOffCmd())
	cmd.AddCommand(newXdebugStatusCmd())
	cmd.AddCommand(newXdebugPauseCmd())
	return cmd
}

func newXdebugOnCmd() *cobra.Command {
	var mode string
	var onDemand bool
	cmd := &cobra.Command{
		Use:   "on [version]",
		Short: "Enable Xdebug for a PHP version (rebuilds the FPM image)",
		Long: "Enable Xdebug for a PHP version. Use --mode to pick a non-default mode, e.g. --mode coverage for code coverage, or --mode debug,coverage to combine.\n\n" +
			"Use --on-demand to set xdebug.start_with_request=trigger: requests and workers no longer auto-connect (no IDE flood); debug a running worker with `lerd xdebug pause`, or a web request via a trigger cookie.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			normalised, err := podman.NormaliseXdebugMode(mode)
			if err != nil {
				return err
			}
			start := "yes"
			if onDemand {
				start = "trigger"
			}
			return runXdebugToggle(args, true, normalised, start)
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "debug", "xdebug.mode value (debug, coverage, develop, profile, trace, gcstats, or a comma-separated combo)")
	cmd.Flags().BoolVar(&onDemand, "on-demand", false, "set start_with_request=trigger so nothing auto-connects; attach with `lerd xdebug pause` or a trigger cookie")
	return cmd
}

func newXdebugOffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "off [version]",
		Short: "Disable Xdebug for a PHP version (rebuilds the FPM image)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runXdebugToggle(args, false, "", "yes")
		},
	}
}

func newXdebugStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show Xdebug status for all installed PHP versions",
		RunE:  runXdebugStatus,
	}
}

func xdebugVersion(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	v, err := phpDet.DetectVersion(cwd)
	if err != nil {
		cfg, err := config.LoadGlobal()
		if err != nil {
			return "", err
		}
		return cfg.PHP.DefaultVersion, nil
	}
	return v, nil
}

func runXdebugToggle(args []string, enable bool, mode, start string) error {
	version, err := xdebugVersion(args)
	if err != nil {
		return err
	}

	applyMode := ""
	if enable {
		applyMode = mode
	}

	res, err := xdebugops.ApplyWithStart(version, applyMode, start)
	if err != nil {
		return err
	}

	feedback.Begin()
	if res.NoChange {
		state := "disabled"
		if res.Enabled {
			state = fmt.Sprintf("enabled (mode=%s)", res.Mode)
		}
		feedback.Line("Xdebug is already " + state + " for PHP " + version)
		return nil
	}

	if res.RestartErr != nil {
		unit := xdebugops.FPMUnit(version)
		feedback.Warn("restart %s: %v", unit, res.RestartErr)
		fmt.Printf("Run: systemctl --user restart %s\n", unit)
	} else if res.Restarted {
		feedback.Note("restarted " + xdebugops.FPMUnit(version))
	}

	// Per-site containers (custom-FPM and FrankenPHP) on this version mount the
	// same xdebug ini; restart them so the toggle takes effect there too.
	podman.RestartSiteContainersForVersion(version)

	if res.Enabled {
		feedback.Done("Xdebug enabled for PHP " + version + " " + feedback.Val(fmt.Sprintf("mode=%s · start=%s · port 9003", res.Mode, start)))
		if start == "trigger" {
			feedback.Note("on-demand: requests and workers won't auto-connect; attach with `lerd xdebug pause` or a trigger cookie")
		}
	} else {
		feedback.Done("Xdebug disabled for PHP " + version)
	}
	return nil
}

func runXdebugStatus(_ *cobra.Command, _ []string) error {
	versions, err := phpDet.ListInstalled()
	if err != nil {
		return fmt.Errorf("listing PHP versions: %w", err)
	}

	if len(versions) == 0 {
		fmt.Println("No PHP versions installed.")
		return nil
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	rows := make([][]string, 0, len(versions))
	for _, v := range versions {
		mode := cfg.GetXdebugMode(v)
		state := feedback.Amber("disabled")
		if mode != "" {
			state = feedback.Green("enabled")
		}
		rows = append(rows, []string{v, state, mode})
	}
	feedback.Table([]string{"Version", "Xdebug", "Mode"}, rows)
	return nil
}
