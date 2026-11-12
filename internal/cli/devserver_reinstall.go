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

// checkoutState is what the last check saw of one checkout.
type checkoutState struct {
	marker time.Time
	// units are the dev servers seen running, kept so one a branch change stops
	// declaring is still restarted onto the new packages.
	units []string
}

// DevServerInstalls notices a JS install under a running dev server, whatever
// ran it, by the marker the package manager writes once the install is done.
type DevServerInstalls struct{ seen map[string]*checkoutState }

func NewDevServerInstalls() *DevServerInstalls {
	return &DevServerInstalls{seen: map[string]*checkoutState{}}
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
		running := runningDevServers(site, path)
		st, known := d.seen[path]
		if !known {
			st = &checkoutState{}
			d.seen[path] = st
		}
		if d.reinstalled(st, path, known) {
			errs = append(errs, restartDevServers(path, union(running, st.units)))
		}
		st.units = running
	}
	return errors.Join(errs...)
}

// reinstalled reports a marker newer than the last one; the first look records,
// "none yet" included. A failed install writes none and is left alone: its
// packages are broken, and the unit writes the config back on its next start.
func (d *DevServerInstalls) reinstalled(st *checkoutState, path string, known bool) bool {
	marker := newestMarker(path)
	prev := st.marker
	if !marker.IsZero() {
		st.marker = marker
	}
	return known && marker.After(prev)
}

func newestMarker(path string) time.Time {
	var newest time.Time
	for _, m := range installMarkers {
		if info, err := os.Stat(filepath.Join(path, m)); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest
}

// runningDevServers names the units of the checkout's declared dev servers that run.
func runningDevServers(site *config.Site, path string) []string {
	fw, ok := config.GetFrameworkForDir(site.Framework, path)
	if !ok || fw == nil {
		return nil
	}
	var units []string
	for name, w := range fw.Workers {
		unit := WorkerUnitName(site.Name, path, name)
		if w.Host && unitIsActiveOrActivating(unit) && config.DevServerToolForWorker(path, w, resolveWorkerCommand(path, name, w)) != nil {
			units = append(units, unit)
		}
	}
	sort.Strings(units)
	return units
}

// restartDevServers restarts the units still running. The files are written back
// first: a unit from before the pre-start step has none to do it.
func restartDevServers(path string, units []string) error {
	if len(units) == 0 {
		return nil
	}
	if err := RestoreDevServerFiles(path); err != nil {
		return err
	}
	var errs []error
	for _, unit := range units {
		if !unitIsActiveOrActivating(unit) {
			continue
		}
		if err := podman.RestartUnit(unit); err != nil {
			errs = append(errs, fmt.Errorf("restarting %s after an install: %w", unit, err))
		}
	}
	return errors.Join(errs...)
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
