package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/podman"
)

// installMarkers are the files package managers write as an install finishes:
// npm, pnpm, yarn 1, yarn berry with and without node_modules, and bun.
var installMarkers = []string{
	"node_modules/.package-lock.json",
	"node_modules/.modules.yaml",
	"node_modules/.yarn-integrity",
	"node_modules/.yarn-state.yml",
	".yarn/install-state.gz",
	"node_modules/.bun-tag",
}

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
// only records it, and no marker at all is an install still running.
func (d *DevServerInstalls) reinstalled(path string) bool {
	var newest time.Time
	for _, m := range installMarkers {
		if info, err := os.Stat(filepath.Join(path, m)); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if newest.IsZero() {
		// A first look mid-install records "none yet", so the marker it writes counts as new.
		if _, known := d.seen[path]; !known {
			d.seen[path] = time.Time{}
		}
		return false
	}
	prev, known := d.seen[path]
	d.seen[path] = newest
	return known && newest.After(prev)
}

// restartRunningDevServers restarts each running dev server of the checkout. The
// files are written back first: a unit from before the pre-start step has none.
func restartRunningDevServers(site *config.Site, path string) error {
	fw, ok := config.GetFrameworkForDir(site.Framework, path)
	if !ok || fw == nil {
		return nil
	}
	if err := RestoreDevServerFiles(path); err != nil {
		return err
	}
	names := make([]string, 0, len(fw.Workers))
	for name := range fw.Workers {
		names = append(names, name)
	}
	sort.Strings(names)
	var errs []error
	for _, name := range names {
		w := fw.Workers[name]
		unit := WorkerUnitName(site.Name, path, name)
		if !w.Host || !unitIsActiveOrActivating(unit) || config.DevServerToolForWorker(path, w, resolveWorkerCommand(path, name, w)) == nil {
			continue
		}
		if err := podman.RestartUnit(unit); err != nil {
			errs = append(errs, fmt.Errorf("restarting %s after an install: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
