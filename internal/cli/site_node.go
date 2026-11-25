package cli

import (
	"fmt"
	"slices"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/linker"
	nodeDet "github.com/geodro/lerd/internal/node"
)

// Seams so tests never reach the machine's own version manager.
var (
	managesNodeFn = func() bool { return lerdManagesNode() && nodeDet.Active().Available() }
	nodeMissingFn = nodeDet.Missing
	installNodeFn = func(v string) error { return nodeDet.Active().Install(v) }
)

// ensureSiteNode installs the Node a site being linked or resumed needs, for it and
// its worktrees. A failure is an error only for a host-proxy site's own version, where
// Node serves the site; any other site works without it.
func ensureSiteNode(site config.Site, r linker.Reporter) error {
	// A custom container brings its own Node.
	if site.IsCustomContainer() || !managesNodeFn() {
		return nil
	}
	dirs := []string{site.Path}
	worktrees, _ := gitpkg.DetectWorktrees(site.Path, site.PrimaryDomain())
	for _, wt := range worktrees {
		dirs = append(dirs, wt.Path)
	}
	var seen []string
	for _, dir := range dirs {
		if !isNodeProject(dir) || bunRunnerFor(dir, false) != "" {
			continue
		}
		v, _ := nodeDet.DetectVersion(dir)
		if slices.Contains(seen, v) || !nodeMissingFn(v) {
			continue
		}
		seen = append(seen, v)
		step := r.Step("installing Node " + v)
		err := installNodeFn(v)
		if err == nil {
			step.OK("")
			continue
		}
		step.Fail(err)
		if site.IsHostProxy() && dir == site.Path {
			return fmt.Errorf("installing Node %s, which %s needs: %w", v, site.Name, err)
		}
		r.Warn("Node %s is not installed, run 'lerd node:install %s'", v, v)
	}
	return nil
}
