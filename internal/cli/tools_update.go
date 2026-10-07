package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/composer"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/tools"
	"github.com/spf13/cobra"
)

// NewToolsUpdateCmd returns the tools:update command.
func NewToolsUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tools:update",
		Short: "Update composer, mise, fnm and mkcert to their pinned versions",
		Long:  "Re-downloads any managed host tool whose installed version differs from the pinned one. Tools that are not installed (e.g. fnm on an nvm-managed setup) are left alone, and so is a mise lerd did not install itself.",
		RunE:  runToolsUpdate,
	}
}

func runToolsUpdate(_ *cobra.Command, _ []string) error {
	feedback.Begin()
	var pins pinnedTools
	updated, failed := 0, 0
	var firstErr error
	for _, s := range tools.StatusAll(context.Background()) {
		if !s.Present {
			feedback.Note(s.Name + " is not installed, skipping")
			continue
		}
		if s.Installed != "" && !s.UpdateAvailable {
			feedback.Line(s.Name + " " + s.Installed + " already matches the pinned version")
			continue
		}
		step := feedback.Start("updating " + s.Name + " to " + s.Pinned)
		// One tool that cannot be updated, a bad pin or a published digest that
		// does not match, must not decide the fate of the others: each is an
		// independent download, so the rest are still attempted and the failure
		// is reported at the end.
		if err := updateToolFn(&pins, s.Name); err != nil {
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
		feedback.Warn("%d of %d tools could not be updated", failed, failed+updated)
		return firstErr
	case updated == 0:
		feedback.Done("all tools are up to date")
	default:
		feedback.Done("tools updated")
	}
	return nil
}

// updateToolFn is the per-tool install, a seam so the loop's failure handling
// can be tested without a network.
var updateToolFn = updateTool

// updateTool reinstalls one tool at its pinned version. fnm and mise go through
// their archive extract; composer and mkcert are plain binary swaps.
func updateTool(pins *pinnedTools, name string) error {
	switch name {
	case "fnm":
		return installFnm(pins, io.Discard)
	case "mise":
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("mise update: %w", err)
		}
		return installMise(pins, home, io.Discard)
	case "mkcert":
		return replaceTool(pins, name, certs.MkcertPath(), io.Discard)
	default:
		return replaceTool(pins, name, composer.PharPath(), io.Discard)
	}
}

// UpdateOneTool reinstalls a single managed tool at its pinned version, for
// callers that apply one at a time and render the outcome themselves. The
// dashboard uses it: each tool card owns its own result, so a checksum that
// rejects one download says so on that card rather than failing a batch.
//
// The name is checked against the managed set rather than trusted, since it
// arrives from a request and decides which path gets written.
func UpdateOneTool(name string) error {
	if !slices.Contains(tools.Names(), name) {
		return fmt.Errorf("unknown tool %q", name)
	}
	if !tools.LerdOwned(name) {
		return fmt.Errorf("%s was not installed by lerd, so lerd leaves it alone", name)
	}
	var pins pinnedTools
	return updateToolFn(&pins, name)
}
