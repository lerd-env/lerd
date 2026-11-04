package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// NewRebuildCmd returns the rebuild command.
func NewRebuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rebuild [site]",
		Short: "Rebuild the custom container image and restart the container",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name, err := resolveSiteName(args)
			if err != nil {
				return err
			}
			return RebuildSite(name)
		},
	}
}

// RebuildSite rebuilds the custom container image from the Containerfile and
// restarts the container. For PHP sites this is a no-op (use php:rebuild).
func RebuildSite(name string) error {
	site, err := config.FindSite(name)
	if err != nil {
		return fmt.Errorf("site %q not found", name)
	}
	if site.IsHostProxy() {
		return fmt.Errorf("site %q is a host-proxy site with no container to rebuild; use 'lerd restart' to restart its dev server", name)
	}
	if !site.IsCustomContainer() && !site.IsCustomFPM() {
		return fmt.Errorf("site %q is not a custom container site, use 'lerd php:rebuild' for PHP sites", name)
	}

	proj, err := config.LoadProjectConfig(site.Path)
	if err != nil {
		return fmt.Errorf("loading .lerd.yaml: %w", err)
	}

	unit := podman.CustomContainerName(name)
	if site.IsCustomFPM() {
		unit = podman.CustomFPMContainerName(name)
	}
	running, _ := podman.ContainerRunning(unit)

	feedback.Begin()
	feedback.Line("building " + podman.CustomImageName(name))
	err = podman.RebuildCustomImage(name, site.Path, proj.Container, func() error {
		if !running {
			return nil
		}
		if err := podman.RestartUnit(unit); err != nil {
			return fmt.Errorf("restarting container: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	podman.StoreContainerfileHash(name, site.Path, proj.Container)

	// Worktree containers run from the same image tag, so a rebuild has to
	// recreate them too or a branch keeps serving the previous image.
	if site.IsCustomContainer() {
		if worktrees, err := gitpkg.ServableWorktrees(site.Path, site.PrimaryDomain()); err == nil {
			for _, wt := range worktrees {
				unit := podman.CustomWorktreeContainerName(name, wt.Branch)
				if !podman.CustomWorktreeQuadletExists(name, wt.Branch) {
					continue
				}
				podman.ResetFailedUnit(unit)
				if err := podman.RestartUnit(unit); err != nil {
					feedback.Warn("restarting worktree container %s: %v", wt.Branch, err)
				}
			}
		}
	}

	feedback.Done("rebuilt and restarted " + feedback.Val(name))
	return nil
}
