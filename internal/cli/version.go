package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/version"
	"github.com/spf13/cobra"
)

// NewVersionCmd returns the version command. It prints exactly what
// `lerd --version` prints: reaching for `lerd version` first is common enough
// that answering it with "unknown command" is the wrong reply to a question
// lerd can answer.
func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the installed lerd version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "lerd version "+version.String())
			return nil
		},
	}
}
