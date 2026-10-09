package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// NewWorkerExecCmd returns the hidden `lerd worker-exec` command a worker unit
// runs on Windows. For an exec-mode worker it is the counterpart of the macOS
// guard script: before it starts the worker's `podman exec`, it ends whatever
// the same worker left running in the container, so a restart or a dropped
// connection never leaves two copies behind. For a host worker (--shell) it
// runs the command line through cmd.exe in --dir with lerd's bin first on
// PATH. It exits with the worker's exit code.
func NewWorkerExecCmd() *cobra.Command {
	var unit, dir string
	var shell bool
	cmd := &cobra.Command{
		Use:          "worker-exec --unit NAME [--dir DIR] [--shell] -- COMMAND [ARGS...]",
		Short:        "Run a worker after clearing its leftovers (internal)",
		Hidden:       true,
		SilenceUsage: true,
		Args:         cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			code, err := runWorkerExec(unit, dir, shell, args)
			if err != nil {
				return err
			}
			os.Exit(code)
			return nil
		},
	}
	cmd.Flags().StringVar(&unit, "unit", "", "worker unit name")
	cmd.Flags().StringVar(&dir, "dir", "", "working directory")
	cmd.Flags().BoolVar(&shell, "shell", false, "run the command line through cmd.exe with lerd's bin first on PATH")
	return cmd
}
