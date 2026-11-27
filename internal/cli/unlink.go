package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/spf13/cobra"
)

// Wire worker teardown into the shared unlink core so every unlink path (CLI,
// MCP, parked-directory watcher) stops the site's workers — including a
// host-proxy site's always-restart dev server.
func init() {
	siteops.RemoveProvidedEnv = dropProvidedEnv
	siteops.StopSiteWorkers = func(site *config.Site) {
		stopWorkersByName(site, collectRunningWorkers(site))
		// collectRunningWorkers only reports active units (correct for pause,
		// which resumes them later). On unlink the site is going away, so tear
		// the dev-server unit down unconditionally — a stopped or failed one
		// would otherwise orphan its .service file. stopWorkerUnit is idempotent.
		if site.IsHostProxy() {
			WorkerStopForSite(site.Name, site.Path, hostProxyWorkerName) //nolint:errcheck
		}
		// Worktrees run their own per-worktree units (lerd-<worker>-<site>-<slug>)
		// that collectRunningWorkers (parent-only) misses. On unlink the site is
		// going away, so stop every worktree's workers too — a host-proxy
		// Restart=always dev server would otherwise loop against a removed dir.
		if wts, err := gitpkg.ServableWorktrees(site.Path, site.PrimaryDomain()); err == nil {
			for _, wt := range wts {
				StopAllWorkersForWorktree(site.Name, filepath.Base(wt.Path)) //nolint:errcheck
			}
		}
		// Path-independent backstop: when the site dir is already gone (watcher
		// prune), worktree detection above finds nothing, so sweep any remaining
		// worker units for this site by name. Idempotent with the loops above.
		stopAllSiteWorkerUnits(site)
	}
}

// UnlinkOptions extends an unlink. DeleteDir also deletes the project directory,
// and Confirmed skips asking first, for callers that already asked.
type UnlinkOptions struct {
	DeleteDir bool
	Confirmed bool
}

// NewUnlinkCmd returns the unlink command.
func NewUnlinkCmd() *cobra.Command {
	var opts UnlinkOptions
	cmd := &cobra.Command{
		Use:   "unlink [name]",
		Short: "Unlink a site, defaulting to the one in the current directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runUnlink(args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.DeleteDir, "delete-files", false, "Also delete the project directory from disk")
	cmd.Flags().BoolVar(&opts.Confirmed, "yes", false, "With --delete-files, delete without asking")
	return cmd
}

func runUnlink(args []string, opts UnlinkOptions) error {
	// Naming the site is the only way to unlink one whose directory has moved
	// or been deleted, and the only way to pick between two registrations that
	// point at the same directory.
	if len(args) == 1 {
		feedback.Begin()
		unlinkErr := UnlinkSiteWith(args[0], opts)
		feedback.Begin()
		return unlinkErr
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	site, err := config.FindSiteByPath(cwd)
	if err != nil {
		return fmt.Errorf("no site registered for %s — link it first with lerd link", cwd)
	}
	feedback.Begin()
	unlinkErr := UnlinkSiteWith(site.Name, opts)
	feedback.Begin()
	return unlinkErr
}

// UnlinkSite removes the nginx vhost for the named site. For sites under a parked
// directory, the registry entry is kept but marked Ignored so the watcher does not
// re-register it. For manually-linked sites the entry is removed entirely.
func UnlinkSite(name string) error {
	return UnlinkSiteWith(name, UnlinkOptions{})
}

// UnlinkSiteWith is UnlinkSite with options. The directory is checked and
// confirmed before anything is unlinked, and deleted only once the unlink is done.
func UnlinkSiteWith(name string, opts UnlinkOptions) error {
	site, err := config.FindSite(name)
	if err != nil {
		return fmt.Errorf("site %q not found", name)
	}
	// The registry stops listing the site once it is unlinked, so the paths that
	// tell the delete which projects lerd mounts are read before that.
	var sitePaths []string
	if opts.DeleteDir {
		if err := confirmDeleteSiteDir(site, opts.Confirmed); err != nil {
			return err
		}
		if reg, err := config.LoadSites(); err == nil {
			for _, s := range reg.Sites {
				sitePaths = append(sitePaths, s.Path)
			}
		}
	}

	// Workers are stopped by UnlinkSiteCore via the siteops.StopSiteWorkers
	// hook registered in this package's init, so every unlink path tears them
	// down uniformly.

	cfg, _ := config.LoadGlobal()
	var parkedDirs []string
	if cfg != nil {
		parkedDirs = cfg.ParkedDirectories
	}

	step := feedback.Start("unlinking " + name)
	if err := siteops.UnlinkSiteCore(site, parkedDirs); err != nil {
		step.Fail(err)
		return err
	}
	step.OK(feedback.Val(site.PrimaryDomain()))

	// Offer to remove the cached custom container image.
	if site.IsCustomContainer() && podman.CustomImageExists(site.Name) {
		if isInteractive() && feedback.Confirm("Remove the container image?", false) {
			_ = podman.RemoveCustomImage(site.Name)
			podman.RemoveContainerfileHash(site.Name)
			feedback.Line("image removed")
		}
	}

	// Offer to remove the framework definition when this was the last site
	// using it and the definition is removable (store-installed or user-defined,
	// never a built-in). It is safe to remove: lerd re-fetches it from the store
	// the moment another site needs it.
	offerRemoveOrphanedFramework(site.Framework)

	// The site is no longer lerd's, so neither is the connection lerd wrote into
	// its IDE configuration.
	removeIDEDataSource(site.Path)

	autoStopUnusedServices()
	autoStopUnusedFPMs()

	if opts.DeleteDir {
		step := feedback.Start("deleting " + site.Path)
		if err := siteops.RemoveSiteDir(site.Name, site.Path, parkedDirs, sitePaths); err != nil {
			step.Fail(err)
			return fmt.Errorf("%s is unlinked, but its directory was not deleted: %w", name, err)
		}
		step.OK("")
	}
	return nil
}

// confirmDeleteSiteDir refuses a directory lerd must not delete, then names what
// goes with it and has the site's name typed back unless the caller confirmed.
func confirmDeleteSiteDir(site *config.Site, confirmed bool) error {
	r := siteops.ReportSiteDir(site.Path, site.PrimaryDomain())
	if r.Refused != "" {
		return fmt.Errorf("not deleting the directory: %s", r.Refused)
	}
	if confirmed {
		return nil
	}
	feedback.Line("this deletes " + r.Path + " from disk")
	for _, wt := range r.InsideWorktrees {
		feedback.Line("  with the worktree " + wt)
	}
	for _, wt := range r.OutsideWorktrees {
		feedback.Warn("the worktree %s stays, but its git data goes with the project", wt)
	}
	for _, dir := range r.Uncommitted {
		feedback.Warn("%s has uncommitted changes", dir)
	}
	if r.Unpushed > 0 {
		feedback.Warn("%d commits are on no remote", r.Unpushed)
	}
	for _, dir := range r.Unchecked {
		feedback.Warn("git could not read %s, so work there may be missing from this list", dir)
	}
	if !isInteractive() {
		return fmt.Errorf("not deleting %s without a terminal to confirm, pass --yes", r.Path)
	}
	if feedback.Ask("Type "+site.Name+" to delete it:") != site.Name {
		return fmt.Errorf("not deleting %s, the name did not match", r.Path)
	}
	return nil
}

// offerRemoveOrphanedFramework prompts, in interactive sessions only, to delete
// a framework definition once no remaining active site references it. Built-in
// frameworks are never offered.
func offerRemoveOrphanedFramework(fw string) {
	if !isInteractive() || !frameworkIsOrphaned(fw) {
		return
	}
	if feedback.Confirm(fmt.Sprintf("No sites use the %q framework anymore. Remove its definition?", fw), false) {
		if err := config.RemoveFramework(fw); err != nil {
			feedback.Warn("could not remove framework %q: %v", fw, err)
			return
		}
		feedback.Line("framework definition removed")
	}
}

// frameworkIsOrphaned reports whether fw has a removable definition (store or
// user, never built-in) that no remaining active site references.
func frameworkIsOrphaned(fw string) bool {
	if fw == "" {
		return false
	}
	for _, name := range config.UnusedInstalledFrameworks() {
		if name == fw {
			return true
		}
	}
	return false
}
