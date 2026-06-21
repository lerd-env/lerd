package cli

import (
	"fmt"
	"os"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewDBIsolateCmd returns the `lerd db:isolate` command.
func NewDBIsolateCmd() *cobra.Command {
	var source string
	cmd := &cobra.Command{
		Use:   "db:isolate",
		Short: "Give the current worktree its own database (cloned from main, another isolated worktree, or empty)",
		Long: `Opt the current worktree into its own database so migrations don't touch
the parent. Run from inside the worktree directory.

The new database is named <parent_db>_<sanitized_branch> in the same
service the parent uses (mysql, mariadb, or postgres). On enable lerd
also rewrites DB_DATABASE in the worktree's .env and persists
db_isolated: true to the worktree's .lerd.yaml so the choice travels
with the branch in git.

The --source flag controls how the new schema is seeded:
  --source empty           Start with an empty database (default)
  --source main            Clone from the parent (mysqldump/pg_dump piped to the new schema)
  --source <branch>        Clone from another already-isolated worktree`,
		RunE: func(_ *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			site, branch, ok := FindParentSiteForWorktree(cwd)
			if !ok {
				return fmt.Errorf("not inside a registered worktree (cwd=%s); run `lerd db:isolate` from the worktree's checkout directory", cwd)
			}
			if err := SetWorktreeDBIsolated(site, branch, true, source); err != nil {
				return err
			}
			feedback.Begin()
			feedback.Done(fmt.Sprintf("worktree %s of %s now uses an isolated database", branch, site.Name))
			return nil
		},
	}
	cmd.Flags().StringVar(&source, "source", "empty", "Initial database contents: empty | main | <branch>")
	return cmd
}

// NewDBShareCmd returns the `lerd db:share` command (the off side of db:isolate).
func NewDBShareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db:share",
		Short: "Drop the current worktree's isolated database and share the parent's again",
		RunE: func(_ *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			site, branch, ok := FindParentSiteForWorktree(cwd)
			if !ok {
				return fmt.Errorf("not inside a registered worktree (cwd=%s)", cwd)
			}
			if err := SetWorktreeDBIsolated(site, branch, false, ""); err != nil {
				return err
			}
			feedback.Begin()
			feedback.Done(fmt.Sprintf("worktree %s of %s now shares the parent's database", branch, site.Name))
			return nil
		},
	}
}
