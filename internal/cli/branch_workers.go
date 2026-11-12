package cli

import (
	"errors"
	"fmt"
	"sort"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// runningHostWorkers names the checkout's active workers that run on the host.
func runningHostWorkers(site *config.Site, fw *config.Framework, path string) []string {
	if site == nil || fw == nil {
		return nil
	}
	var names []string
	for name, w := range fw.Workers {
		if w.Host && unitIsActiveOrActivating(WorkerUnitName(site.Name, path, name)) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// restartAfterJSInstall restarts the host workers that ran before the install:
// npm ci replaced node_modules under them and deleted the dev server config
// lerd generates there, which the worker start writes back first.
func restartAfterJSInstall(site *config.Site, fw *config.Framework, path string, names []string) error {
	php := ""
	if site != nil {
		php = config.WorktreePHPVersion(path, site.PHPVersion)
	}
	var errs []error
	for _, name := range names {
		if fw != nil {
			if w, ok := fw.Workers[name]; ok {
				if err := WorkerStartForSite(site.Name, path, php, name, w, false); err != nil {
					errs = append(errs, fmt.Errorf("restarting %s after the install: %w", name, err))
					continue
				}
			}
		}
		// The start is a no-op on a unit that is still running, so the restart
		// is what puts it on the new packages.
		if err := podman.RestartUnit(WorkerUnitName(site.Name, path, name)); err != nil {
			errs = append(errs, fmt.Errorf("restarting %s after the install: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
