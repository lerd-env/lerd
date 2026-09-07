package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/licenses"
	"github.com/spf13/cobra"
)

// NewLicensesCmd returns the licenses command.
func NewLicensesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "licenses",
		Short: "Show the third-party license notices bundled with lerd",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), licenses.Notices())
			return err
		},
	}
}
