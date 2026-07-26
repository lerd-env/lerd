package cli

import (
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	nodeDet "github.com/geodro/lerd/internal/node"
	"github.com/spf13/cobra"
)

// NewNodeUseCmd returns the node:use command.
func NewNodeUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "node:use <version>",
		Short: "Set the default Node.js version",
		Args:  cobra.ExactArgs(1),
		RunE:  runNodeUse,
	}
}

func runNodeUse(_ *cobra.Command, args []string) error {
	if err := ensureNodeManaged(); err != nil {
		return err
	}
	major := strings.SplitN(args[0], ".", 2)[0]

	if err := nodeDet.Active().SetDefault(major); err != nil {
		return err
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	cfg.Node.DefaultVersion = major
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}

	feedback.Begin()
	feedback.Done("default Node set to " + feedback.Val(major))
	return nil
}
