package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/podman"
)

// GenerateCustomContainerWorktreeVhost writes the branch vhost. Split from
// starting the container so the watcher's boot scan can route every worktree
// subdomain before the (slower) container start.
func GenerateCustomContainerWorktreeVhost(site config.Site, wt gitpkg.Worktree) error {
	return nginx.GenerateWorktreeCustomVhostFor(wt.Domain, wt.Path, site.PrimaryDomain(), site.Name, wt.Branch, site.Secured)
}

// EnsureCustomContainerWorktreeContainer starts the branch container without
// touching the vhost. Used by the boot reconcile, where routing is written
// synchronously and the runtime starts afterwards.
func EnsureCustomContainerWorktreeContainer(site config.Site, wt gitpkg.Worktree) error {
	return podman.EnsureCustomWorktreeContainer(site.Name, wt.Branch, wt.Path, site.ContainerPort)
}

// SetupCustomContainerWorktree starts the branch container and points the
// branch domain at it. Called on the watcher's "added" event so the first
// request after `git worktree add` reaches the branch checkout.
func SetupCustomContainerWorktree(site config.Site, wt gitpkg.Worktree) error {
	if err := EnsureCustomContainerWorktreeContainer(site, wt); err != nil {
		return fmt.Errorf("starting worktree container: %w", err)
	}
	return GenerateCustomContainerWorktreeVhost(site, wt)
}

// CleanupStaleCustomWorktreeContainers removes branch containers that no
// longer have a checkout. Safe to call for non-custom sites and for sites
// without worktrees.
func CleanupStaleCustomWorktreeContainers(site *config.Site) {
	if site == nil || !site.IsCustomContainer() {
		return
	}
	worktrees, err := gitpkg.ServableWorktrees(site.Path, site.PrimaryDomain())
	if err != nil {
		return
	}
	branches := make([]string, 0, len(worktrees))
	for _, wt := range worktrees {
		branches = append(branches, wt.Branch)
	}
	podman.CleanupStaleCustomWorktreeContainers(site.Name, branches)
}
