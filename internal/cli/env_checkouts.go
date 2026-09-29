package cli

import (
	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
)

// envCheckoutPaths lists every checkout whose env belongs to one site: the main
// one, then each worktree. A worktree keeps its own env file and is not a site
// in the registry, so a sweep that walks the registry alone rewrites the main
// checkout and leaves the worktree on whatever it was wired to when it was
// created, which on a loopback runtime is a container name that resolves
// nowhere. A worktree whose checkout is gone is skipped rather than handed to
// a command that would run in no directory.
func envCheckoutPaths(sitePath string, worktrees []gitpkg.Worktree) []string {
	paths := []string{sitePath}
	for _, wt := range worktrees {
		if wt.Path == "" {
			continue
		}
		paths = append(paths, wt.Path)
	}
	return paths
}

// siteEnvCheckouts is envCheckoutPaths with the worktrees read from git. A
// checkout that is not a repository, or one git cannot be asked about, leaves
// the site itself, which is what the sweeps did before.
func siteEnvCheckouts(site *config.Site) []string {
	worktrees, err := gitpkg.DetectWorktrees(site.Path, site.PrimaryDomain())
	if err != nil {
		return []string{site.Path}
	}
	return envCheckoutPaths(site.Path, worktrees)
}
