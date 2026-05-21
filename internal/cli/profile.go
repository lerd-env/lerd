package cli

import (
	"fmt"
	"os"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/profiler"
	"github.com/spf13/cobra"
)

// NewProfileCmd returns the parent `lerd profile` command. Subcommands turn the
// global SPX profiler on and off, show its state, and open its web UI.
func NewProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Profile your sites with the SPX profiler",
		Long: `Turn the SPX profiler on or off. While it is on, every HTTP request to
every PHP-FPM site is profiled; browse the flame graphs in the dashboard
Profiler view. Toggling injects an SPX cookie via nginx, with no FPM restart.`,
	}
	cmd.AddCommand(newProfileOnCmd())
	cmd.AddCommand(newProfileOffCmd())
	cmd.AddCommand(newProfileStatusCmd())
	cmd.AddCommand(newProfileOpenCmd())
	cmd.AddCommand(newProfileRunCmd())
	cmd.AddCommand(newProfileClearCmd())
	return cmd
}

func newProfileOnCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "on",
		Short: "Turn the SPX profiler on (every PHP-FPM site is profiled)",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return runProfileToggle(true) },
	}
}

func newProfileOffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "off",
		Short: "Turn the SPX profiler off",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return runProfileToggle(false) },
	}
}

func newProfileStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the SPX profiler is on",
		Args:  cobra.NoArgs,
		RunE:  runProfileStatus,
	}
}

func newProfileClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Delete all captured SPX profile reports",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			n, err := profiler.ClearData()
			if err != nil {
				return err
			}
			if n == 0 {
				fmt.Println("No profile reports to clear.")
			} else {
				fmt.Printf("Cleared %d profile report(s).\n", n)
			}
			return nil
		},
	}
}

func newProfileOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open",
		Short: "Open the SPX profiler web UI in the browser",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return openBrowser(profiler.SpxUIURL) },
	}
}

func runProfileToggle(on bool) error {
	res, err := profiler.SetProfiling(on)
	if err != nil {
		return err
	}
	switch {
	case res.NoChange && on:
		fmt.Println("Profiler already on.")
	case res.NoChange:
		fmt.Println("Profiler already off.")
	case on:
		fmt.Println("Profiler on. Every PHP-FPM site is now profiled. Open the Profiler view for flame graphs.")
	default:
		fmt.Println("Profiler off.")
	}
	return nil
}

func runProfileStatus(_ *cobra.Command, _ []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	state, colour := "off", colorYellow
	if cfg.IsProfilerEnabled() {
		state, colour = "on", colorGreen
	}
	fmt.Printf("Profiler:   %s%s%s\n", colour, state, colorReset)
	fmt.Printf("SPX web UI: %s\n", profiler.SpxUIURL)
	return nil
}

func newProfileRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <command> [args...]",
		Short: "Profile a one-off CLI command (e.g. lerd profile run artisan queue:work)",
		Long: `Run a PHP CLI command with SPX profiling enabled. The command is executed
as 'php <command> [args...]' inside the project's container; the resulting
report shows up in the Profiler view alongside HTTP-request reports. Useful
for artisan commands, queue jobs, and test runs.`,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE:               runProfileRun,
	}
}

func runProfileRun(_ *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: lerd profile run <command> [args...]")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "[lerd] profiling this run with SPX, the report will appear in the Profiler view")
	code, err := RunPHPCaptureEnv(cwd, args, []string{"SPX_ENABLED=1", "SPX_REPORT=full"})
	if err != nil {
		return err
	}
	if code != 0 {
		os.Exit(code)
	}
	return nil
}
