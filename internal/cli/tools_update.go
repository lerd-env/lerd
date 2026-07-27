package cli

import (
	"context"
	"io"
	"path/filepath"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/tools"
	"github.com/spf13/cobra"
)

// NewToolsUpdateCmd returns the tools:update command.
func NewToolsUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tools:update",
		Short: "Update composer, fnm and mkcert to their pinned versions",
		Long:  "Re-downloads any managed host tool whose installed version differs from the pinned one. Tools that are not installed (e.g. fnm on an nvm-managed setup) are left alone.",
		RunE:  runToolsUpdate,
	}
}

func runToolsUpdate(_ *cobra.Command, _ []string) error {
	feedback.Begin()
	var pins pinnedTools
	updated := 0
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
		if err := updateTool(&pins, s.Name); err != nil {
			step.Fail(err)
			return err
		}
		step.OK("")
		updated++
	}
	if updated == 0 {
		feedback.Done("all tools are up to date")
	} else {
		feedback.Done("tools updated")
	}
	return nil
}

// updateTool reinstalls one tool at its pinned version. fnm goes through the
// zip extract; composer and mkcert are plain binary swaps.
func updateTool(pins *pinnedTools, name string) error {
	switch name {
	case "fnm":
		return installFnm(pins, io.Discard)
	case "mkcert":
		return replaceTool(pins, name, certs.MkcertPath(), io.Discard)
	default:
		return replaceTool(pins, name, filepath.Join(config.BinDir(), "composer.phar"), io.Discard)
	}
}
