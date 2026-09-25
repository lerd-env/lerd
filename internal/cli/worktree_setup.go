package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/geodro/lerd/internal/config"
	"github.com/spf13/cobra"
)

// newWorktreeSetupCmd finishes an installed worktree without prompting. The MCP
// server calls it after add, since an assistant has no terminal to answer
// `lerd worktree add`'s prompts and a tree with deps alone fails its first request.
func newWorktreeSetupCmd() *cobra.Command {
	var build, db string
	cmd := &cobra.Command{
		Use:    "setup",
		Short:  "Finish the current worktree's setup without prompting (asset build, database, framework commands)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			site, branch, ok := FindParentSiteForWorktree(cwd)
			if !ok {
				return fmt.Errorf("not inside a lerd worktree (cwd=%s)", cwd)
			}
			return RunWorktreeSetup(site, cwd, branch, build, db, os.Stdout)
		},
	}
	cmd.Flags().StringVar(&build, "build", "auto", "auto | skip | worker:<name> | script:<name>")
	cmd.Flags().StringVar(&db, "db", "", "share | empty | clone-main | clone-<branch> | reuse | reset (default share)")
	return cmd
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
