package cli

import (
	"github.com/geodro/lerd/internal/feedback"
	nodeDet "github.com/geodro/lerd/internal/node"
	"github.com/spf13/cobra"
)

// NewNodeInstallCmd returns the node:install command.
func NewNodeInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "node:install <version>",
		Short: "Install a Node.js version",
		Args:  cobra.ExactArgs(1),
		RunE:  runNodeInstall,
	}
}

func runNodeInstall(_ *cobra.Command, args []string) error {
	if err := ensureNodeManaged(); err != nil {
		return err
	}
	version := args[0]
	mgr := nodeDet.Active()

	feedback.Begin()
	step := feedback.Start("installing Node " + version)
	if err := mgr.Install(version); err != nil {
		step.Fail(err)
		return err
	}
	step.OK("")

	// Set as default if no default is configured yet.
	if !mgr.HasDefault() {
		_ = mgr.SetDefault(version)
	}
	return nil
}
