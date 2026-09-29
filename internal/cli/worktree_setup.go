package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// newWorktreeSetupCmd finishes a worktree without prompting: the half of
// `lerd worktree add` that comes after git, for worktrees another tool created
// with plain git. The MCP server calls it after add too, since an assistant has
// no terminal to answer the prompts.
func newWorktreeSetupCmd() *cobra.Command {
	var build, db string
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "setup [path]",
		Short: "Wait for a worktree's install, then run its asset build, database setup and framework commands",
		Long: `Finish a worktree created with plain git (by an editor, an agent tool or a
script) the way lerd worktree add finishes its own: wait for the watcher's
install, then run the asset build, pick and migrate the database, and run the
framework's worktree setup commands. Nothing is prompted.

Without a path, the current directory is used. Exits 3 when the path is not a
worktree lerd manages, and 1 when the install does not settle in time.`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			}
			err := SetupManagedWorktree(path, build, db, timeout, os.Stdout)
			if errors.Is(err, ErrNotLerdWorktree) {
				feedback.Fail(err)
				os.Exit(exitNotLerdWorktree)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&build, "build", "auto", "auto | skip | worker:<name> | script:<name>")
	cmd.Flags().StringVar(&db, "db", "", "share | empty | clone-main | clone-<branch> | reuse | reset (default: picked from the migrations)")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "How long to wait for the install before giving up")
	return cmd
}

// SetupManagedWorktree waits for path's install to settle, then finishes it.
// Nothing runs on a timeout: building or migrating mid-install is the race the
// wait exists to prevent.
func SetupManagedWorktree(path, build, db string, timeout time.Duration, log io.Writer) error {
	if err := WaitForManagedWorktree(path, timeout); err != nil {
		return err
	}
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	site, branch, ok := FindParentSiteForWorktree(abs)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotLerdWorktree, abs)
	}
	return RunWorktreeSetup(site, abs, branch, build, db, log)
}

// RunWorktreeSetup runs the steps `lerd worktree add` prompts for, with the
// dashboard's request values, once the worktree's dependencies are installed.
func RunWorktreeSetup(site *config.Site, worktreePath, branch, build, db string, log io.Writer) error {
	if !site.IsHostProxy() {
		applyWorktreeBuildRequest(site, worktreePath, build, log)
	}
	fw, hasFramework := config.GetFrameworkForDir(site.Framework, site.Path)
	choice, reason, needsMigrate := planUnattendedWorktreeDB(fw, db, site.Path, worktreePath, WorktreeUsesSQLite(site))
	if reason != "" {
		logf(log, "Database: %s, since %s.", choice, reason)
	}
	if err := ApplyWorktreeDBChoice(site, branch, choice, log); err != nil {
		return fmt.Errorf("database setup: %w", err)
	}
	if migrate := worktreeMigrateCommand(fw); migrate != "" && needsMigrate {
		logf(log, "Running %s...", migrate)
		cmd := exec.Command("sh", "-c", migrate)
		cmd.Dir = worktreePath
		cmd.Env = append(os.Environ(), "PATH="+config.PathWithBinDir())
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", migrate, err)
		}
	}
	if hasFramework {
		runWorktreeSetupCommands(fw, worktreePath, log)
	}
	return nil
}
