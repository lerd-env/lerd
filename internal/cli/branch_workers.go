package cli

import (
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/podman"
)

// installJSDeps is the JS install itself, indirected so tests need no npm.
var installJSDeps = gitpkg.InstallJS

// installJSWithWorkersStopped stops the checkout's running host workers around
// the JS install: npm ci replaces node_modules, stale under a running dev server,
// and deletes the config lerd generates there, which a fresh start writes back.
func installJSWithWorkersStopped(site *config.Site, fwAfter func() *config.Framework, path string, out io.Writer) error {
	if site == nil {
		return installJSDeps(path, out)
	}
	fw := fwAfter()
	running := runningHostWorkers(site, fw, path)
	for i, name := range running {
		if err := podman.StopUnit(WorkerUnitName(site.Name, path, name)); err != nil {
			startWorkersAgain(site, fw, path, running[:i])
			return fmt.Errorf("stopping %s before the install: %w", name, err)
		}
	}
	return errors.Join(installJSDeps(path, out), startWorkersAgain(site, fw, path, running))
}

// runningHostWorkers names the checkout's active workers that run on the host.
func runningHostWorkers(site *config.Site, fw *config.Framework, path string) []string {
	if fw == nil {
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

func startWorkersAgain(site *config.Site, fw *config.Framework, path string, names []string) error {
	php := config.WorktreePHPVersion(path, site.PHPVersion)
	var errs []error
	for _, name := range names {
		if err := WorkerStartForSite(site.Name, path, php, name, fw.Workers[name], false); err != nil {
			errs = append(errs, fmt.Errorf("starting %s again: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
