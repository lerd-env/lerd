package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// NewWorkerExecCmd returns the hidden `lerd worker-exec` command an exec-mode
// worker unit runs on Windows, the counterpart of the macOS guard script:
// before it starts the worker's `podman exec`, it ends whatever the same worker
// left running in the container, so a restart or a dropped connection never
// leaves two copies behind. It exits with the worker's exit code.
func NewWorkerExecCmd() *cobra.Command {
	var unit string
	cmd := &cobra.Command{
		Use:          "worker-exec --unit NAME -- COMMAND [ARGS...]",
		Short:        "Run an exec-mode worker after clearing its leftovers (internal)",
		Hidden:       true,
		SilenceUsage: true,
		Args:         cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			code, err := runWorkerExec(unit, args)
			if err != nil {
				return err
			}
			os.Exit(code)
			return nil
		},
	}
	cmd.Flags().StringVar(&unit, "unit", "", "worker unit name")
	return cmd
}
