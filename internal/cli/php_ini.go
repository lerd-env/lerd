package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/phpini"
	"github.com/spf13/cobra"
)

// NewPhpIniCmd returns the php:ini command.
func NewPhpIniCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "php:ini [version|shared]",
		Short: "Edit the user php.ini for a PHP version, or the shared file (php:ini shared)",
		Long: "Edit a PHP version's php.ini, or the shared file applied to every version.\n\n" +
			"  lerd php:ini            detected/default version\n" +
			"  lerd php:ini 8.4        an explicit version\n" +
			"  lerd php:ini shared     the shared file (all versions; a per-version key still wins)",
		Args: cobra.MaximumNArgs(1),
		RunE: runPhpIni,
	}
}

func runPhpIni(_ *cobra.Command, args []string) error {
	scope := phpini.SharedScope
	if len(args) != 1 || args[0] != phpini.SharedScope {
		v, err := phpExtVersion(args)
		if err != nil {
			return err
		}
		scope = v
	}

	if err := phpini.Ensure(scope); err != nil {
		return fmt.Errorf("creating ini: %w", err)
	}

	path := phpini.ScopeFile(scope).Path
	launched, err := launchEditor(path)
	if err != nil {
		return err
	}
	if !launched {
		feedback.Begin()
		feedback.Line("ini file: " + feedback.Val(path))
		feedback.Note("set $EDITOR to open it automatically")
		return nil
	}

	feedback.Begin()
	label := scope
	if scope == phpini.SharedScope {
		label = "shared (all versions)"
	}
	step := feedback.Start("applying " + label)
	if err := phpini.Restart(scope); err != nil {
		step.Fail(err)
		return fmt.Errorf("applying ini change: %w", err)
	}
	step.OK("")
	return nil
}
