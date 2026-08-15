package cli

import (
	"github.com/spf13/cobra"
)

// NewCheckCmd returns the check command. Validating .lerd.yaml is now one check
// inside the site doctor, so this stays only as an alias for the muscle memory
// and points the user at the command that answers the whole question.
func NewCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "check",
		Short:        "Validate .lerd.yaml (alias for lerd site:doctor)",
		Deprecated:   "use `lerd site:doctor`, which validates .lerd.yaml as part of the site's health report.",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runSiteDoctor("", false, false)
		},
	}
}
