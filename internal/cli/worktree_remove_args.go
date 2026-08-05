package cli

import (
	"strings"

	gitpkg "github.com/geodro/lerd/internal/git"
)

// resolveWorktreeArgs maps a branch name in the positional argument to the
// checkout git actually knows about. `lerd worktree add -b feat-x` creates
// <project>-feat-x, so removing by the branch name is the symmetric command a
// user reaches for, but git only accepts a path or the checkout basename and
// answers "'feat-x' is not a working tree". Anything that is a flag, already a
// path, or matches no worktree is passed through for git to judge.
func resolveWorktreeArgs(sitePath string, args []string) []string {
	worktrees, err := gitpkg.DetectWorktrees(sitePath, "")
	if err != nil || len(worktrees) == 0 {
		return args
	}

	out := make([]string, len(args))
	copy(out, args)
	for i, a := range out {
		if strings.HasPrefix(a, "-") || strings.Contains(a, "/") {
			continue
		}
		for _, wt := range worktrees {
			// The basename is what git already accepts, so only a name that is
			// purely the branch needs rewriting.
			if wt.Name == a {
				break
			}
			if wt.Branch == a || gitpkg.SanitizeBranch(a) == wt.Branch {
				out[i] = wt.Path
				break
			}
		}
	}
	return out
}
