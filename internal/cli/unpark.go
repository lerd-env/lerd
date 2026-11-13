package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/spf13/cobra"
)

// The teardown seams tests replace, so an unpark can be exercised without a
// container runtime: the tail rewrites quadlets and container hosts, which a
// unit test has no business doing.
var (
	teardownSiteFn      = siteops.TeardownSite
	finishSiteRemovalFn = siteops.FinishSiteRemoval
)

// NewUnparkCmd returns the unpark command.
func NewUnparkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpark [directory]",
		Short: "Remove a parked directory and unlink all its sites",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runUnpark,
	}
}

func runUnpark(_ *cobra.Command, args []string) error {
	dir := ""
	if len(args) > 0 {
		dir = args[0]
	} else {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	// Remove from parked directories list
	found := false
	filtered := cfg.ParkedDirectories[:0]
	for _, pd := range cfg.ParkedDirectories {
		if config.SamePath(pd, absDir) {
			found = true
		} else {
			filtered = append(filtered, pd)
		}
	}
	if !found {
		return fmt.Errorf("%s is not a parked directory", absDir)
	}
	cfg.ParkedDirectories = filtered
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}

	// Remove all sites whose path is under this directory
	reg, err := config.LoadSites()
	if err != nil {
		return err
	}

	// Through the shared teardown rather than dropping the vhost and the
	// registry entry by hand: that left every site's workers running, and a
	// queue worker whose project is no longer a site restart-loops for as long
	// as the machine is up. The teardown also stops shares, drops the worktree
	// vhosts and the certificates, and forgets the site's recorded state.
	// The parked list is saved without this directory above, so IsParkedSite is
	// already false here and the sites are removed rather than ignored.
	feedback.Begin()
	removed := 0
	for _, site := range reg.Sites {
		if !underDir(site.Path, absDir) {
			continue
		}
		teardownSiteFn(&site, cfg.ParkedDirectories)
		feedback.Start("unlinking " + site.Name).OK(feedback.Val(site.PrimaryDomain()))
		removed++
	}

	feedback.Done(fmt.Sprintf("unparked %s · %d site(s) removed", filepath.Base(absDir), removed))

	// The install-wide tail runs once for the whole batch rather than per site.
	if err := finishSiteRemovalFn(); err != nil {
		feedback.Warn("reloading nginx: %v", err)
	}

	return nil
}

// underDir reports whether path lies inside dir, asking the filesystem rather
// than comparing strings: sites are stored resolved while dir comes as typed,
// through an ostree /home link or in another case on a macOS volume.
func underDir(path, dir string) bool {
	for p := filepath.Dir(path); p != filepath.Dir(p); p = filepath.Dir(p) {
		if config.SamePath(p, dir) {
			return true
		}
	}
	return false
}
