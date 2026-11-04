package cli

import (
	"context"
	"fmt"

	"github.com/geodro/lerd/internal/feedback"
	nodeDet "github.com/geodro/lerd/internal/node"
	"github.com/spf13/cobra"
)

// Seams so the update loop can be tested without a network or a manager.
var (
	nodeStatusesFn = nodeDet.Statuses
	nodeUpdateFn   = nodeDet.Update
)

// NewNodeUpdateCmd returns the node:update command.
func NewNodeUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "node:update [major]",
		Short: "Install the newest patch of the Node.js versions in use",
		Long:  "Installs the newest published patch of one installed major, or of every major that has one. Older patches stay installed, and running dev servers keep their Node until they restart.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runNodeUpdate,
	}
}

func runNodeUpdate(cmd *cobra.Command, args []string) error {
	if err := ensureNodeManaged(); err != nil {
		return err
	}
	major := ""
	if len(args) == 1 {
		major = args[0]
	}
	return updateNodeVersions(cmd.Context(), major)
}

func updateNodeVersions(ctx context.Context, major string) error {
	feedback.Begin()
	var targets []nodeDet.VersionStatus
	for _, s := range nodeStatusesFn(ctx) {
		if major == "" || s.Major == major {
			targets = append(targets, s)
		}
	}
	if len(targets) == 0 && major != "" {
		return fmt.Errorf("Node %s is not installed", major)
	}

	updated, failed := 0, 0
	var firstErr error
	for _, s := range targets {
		if s.Latest == "" {
			if firstErr == nil {
				firstErr = fmt.Errorf("could not read the published Node releases for %s", s.Major)
			}
			failed++
			continue
		}
		if !s.UpdateAvailable {
			feedback.Line("Node " + s.Installed + " is the newest " + s.Major)
			continue
		}
		step := feedback.Start("updating Node " + s.Installed + " to " + s.Latest)
		// One version that cannot be downloaded must not cost the others theirs.
		if _, err := nodeUpdateFn(ctx, s.Major); err != nil {
			step.Fail(err)
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		step.OK("")
		updated++
	}
	switch {
	case failed > 0:
		feedback.Warn("%d of %d Node versions could not be updated", failed, failed+updated)
		return firstErr
	case updated == 0:
		feedback.Done("Node is up to date")
	default:
		feedback.Done("Node updated")
	}
	return nil
}
