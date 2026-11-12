package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/podman"
)

// DevServerInstalls notices a JS install under a running dev server, whatever
// ran it, by the marker the package manager writes once the install is done.
type DevServerInstalls struct{ seen map[string]time.Time }

func NewDevServerInstalls() *DevServerInstalls {
	return &DevServerInstalls{seen: map[string]time.Time{}}
}

// Check restarts the running dev servers of the site's checkouts, its own and
// its worktrees', whose packages were installed again since the last check.
func (d *DevServerInstalls) Check(site *config.Site) error {
	paths := []string{site.Path}
	if wts, err := gitpkg.DetectWorktrees(site.Path, site.PrimaryDomain()); err == nil {
		for _, wt := range wts {
			paths = append(paths, wt.Path)
		}
	}
	var errs []error
	for _, path := range paths {
		if d.reinstalled(path) {
			errs = append(errs, restartRunningDevServers(site, path))
		}
	}
	return errors.Join(errs...)
}

// reinstalled reports a marker newer than the one seen last. The first look
// only records it, and a missing marker is an install still running.
func (d *DevServerInstalls) reinstalled(path string) bool {
	info, err := os.Stat(gitpkg.JSInstallMarker(path))
	if err != nil {
		return false
	}
	prev, known := d.seen[path]
	d.seen[path] = info.ModTime()
	return known && info.ModTime().After(prev)
}

// restartRunningDevServers stops each running dev server of the checkout and
// starts it again: the start writes back the config the install deleted, and
// on a unit still running it would be a no-op.
func restartRunningDevServers(site *config.Site, path string) error {
	fw, ok := config.GetFrameworkForDir(site.Framework, path)
	if !ok || fw == nil {
		return nil
	}
	names := make([]string, 0, len(fw.Workers))
	for name := range fw.Workers {
		names = append(names, name)
	}
	sort.Strings(names)
	php := config.WorktreePHPVersion(path, site.PHPVersion)
	var errs []error
	for _, name := range names {
		w := fw.Workers[name]
		unit := WorkerUnitName(site.Name, path, name)
		if !w.Host || !unitIsActiveOrActivating(unit) || config.DevServerToolForWorker(path, w, resolveWorkerCommand(path, name, w)) == nil {
			continue
		}
		if err := podman.StopUnit(unit); err != nil {
			errs = append(errs, fmt.Errorf("restarting %s after an install: %w", name, err))
			continue
		}
		if err := WorkerStartForSite(site.Name, path, php, name, w, false); err != nil {
			errs = append(errs, fmt.Errorf("restarting %s after an install: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
