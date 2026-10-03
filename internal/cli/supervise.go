package cli

import (
	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/services"
)

// NewSuperviseCmd returns the hidden `lerd supervise` command, which the
// Windows service manager runs a unit under so the unit's restart policy is
// honoured: it runs the command after --, waits for it, and starts it again on
// exit as --restart says.
func NewSuperviseCmd() *cobra.Command {
	var unit, restart string
	cmd := &cobra.Command{
		Use:          "supervise --unit NAME --restart always|on-failure -- COMMAND [ARGS...]",
		Short:        "Run a service unit and restart it by its policy (internal)",
		Hidden:       true,
		SilenceUsage: true,
		Args:         cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return services.Supervise(unit, restart, args)
		},
	}
	cmd.Flags().StringVar(&unit, "unit", "", "unit name, for the log")
	cmd.Flags().StringVar(&restart, "restart", "on-failure", "restart policy: always, on-failure or no")
	return cmd
}
