package cli

import (
	"github.com/spf13/cobra"
)

// NewP9GuardCmd returns the hidden `lerd p9-guard` command, which runs
// `lerd p9-serve` with the arguments after -- and, should the server die while
// the machine is up, starts it again and brings the VM's mounts and lerd's
// containers back onto it.
func NewP9GuardCmd() *cobra.Command {
	var machine string
	cmd := &cobra.Command{
		Use:          "p9-guard --machine NAME -- --serve DIR:GUID... PID",
		Short:        "Keep lerd's 9p server up while the machine runs (internal)",
		Hidden:       true,
		SilenceUsage: true,
		Args:         cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return runP9Guard(machine, args)
		},
	}
	cmd.Flags().StringVar(&machine, "machine", "", "Podman machine name")
	return cmd
}
