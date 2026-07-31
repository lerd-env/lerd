package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// ErrNotLerdWorktree marks a path lerd does not manage as a worktree, which
// callers have to tell apart from a timeout: one means "ask someone else", the
// other means "still going".
var ErrNotLerdWorktree = errors.New("not a worktree lerd manages")

// exitNotLerdWorktree is the distinct status for ErrNotLerdWorktree. 1 stays
// the timeout, and 2 is left alone because cobra spends it on usage errors.
const exitNotLerdWorktree = 3

// newWorktreeWaitCmd is the non-interactive half of `lerd worktree add`'s wait,
// for tools that create a worktree with plain git and need to know when lerd has
// finished provisioning it before they touch the tree themselves.
func newWorktreeWaitCmd() *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "wait [path]",
		Short: "Block until lerd's setup pipeline has finished for a worktree",
		Long: `Block until lerd's watcher-driven setup pipeline has finished for a worktree,
so a tool that created one with plain git can wait before acting on the tree.

Without a path, the current directory is used. Nothing is printed on success;
the exit status is the contract:

  0  the worktree is provisioned and no install is running
  1  the timeout elapsed first
  3  this path is not a worktree lerd manages

Waiting covers both the pipeline's outputs (.env, vendor/autoload.php,
node_modules) and its install lock, because node_modules exists from the first
extracted package and would otherwise read as finished mid-install.`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			}
			err := WaitForManagedWorktree(path, timeout)
			if errors.Is(err, ErrNotLerdWorktree) {
				feedback.Fail(err)
				os.Exit(exitNotLerdWorktree)
			}
			return err
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "How long to wait before giving up")
	return cmd
}

// WaitForManagedWorktree resolves path (empty means the current directory),
// confirms lerd manages it as a worktree, then blocks until its setup pipeline
// has settled. Rejecting an unmanaged path up front matters: nothing would ever
// provision it, so waiting could only ever end in a timeout.
func WaitForManagedWorktree(path string, timeout time.Duration) error {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		path = cwd
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, ok := config.ParentSiteForWorktreeDir(abs); !ok {
		return fmt.Errorf("%w: %s", ErrNotLerdWorktree, abs)
	}
	return WaitForWorktreeReady(abs, timeout)
}
