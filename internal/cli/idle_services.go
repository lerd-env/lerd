package cli

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/lifecycle"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/siteops"
)

// The service operations idle-suspend drives, vars so tests can stand in for
// the containers and nginx.
var (
	idleStopService          = serviceops.StopWithDependents
	idleEnsureService        = serviceops.WakeService
	idleServiceUp            = serviceUnitUp
	idleSitesUsing           = config.SitesUsingService
	idleSwapToWaking         = swapSiteToWaking
	idleRestoreVhost         = func(s *config.Site) error { return siteops.RegenerateSiteVhost(s, s.PrimaryDomain()) }
	idleReloadNginx          = func() { nginx.ReloadOrWarn("") }
	idleDependentsOf         = serviceops.DependentsOf
	idleAdminToolsFor        = serviceops.AdminToolsFor
	idleDiscoveringConsumers = serviceops.DiscoveringConsumers
	idleServiceFlagged       = config.ServiceIsIdleSuspended
)

// RunningServicesForIdle lists the installed, unpaused services whose unit is
// up, the only ones idle-suspend can put to sleep.
func RunningServicesForIdle() []string {
	var out []string
	for _, unit := range lifecycle.InstalledServiceUnits() {
		name := strings.TrimPrefix(unit, "lerd-")
		if idleServiceUp(name) {
			out = append(out, name)
		}
	}
	return out
}

// IdleServiceUsers returns the sites using a service and the services that rely
// on it (dependents, admin tools administering it, services discovering it
// through their env), whose own users keep it awake.
func IdleServiceUsers(name string) (sites []config.Site, consumers []string) {
	seen := map[string]bool{}
	for _, list := range [][]string{idleDependentsOf(name), idleAdminToolsFor(name), idleDiscoveringConsumers(name)} {
		for _, c := range list {
			if !seen[c] {
				seen[c] = true
				consumers = append(consumers, c)
			}
		}
	}
	return idleSitesUsing(name), consumers
}

// SuspendServiceForIdle puts a service to sleep: the sites using it get the
// waking page first, so a request never reaches the app while its service is
// down, then the service and its running dependents stop. Returns every
// service it stopped, for the caller to hold asleep.
func SuspendServiceForIdle(name string) ([]string, error) {
	stopped := []string{name}
	for _, dep := range idleDependentsOf(name) {
		if idleServiceUp(dep) {
			stopped = append(stopped, dep)
		}
	}
	sites := idleSitesUsing(name)
	for i := range sites {
		if err := idleSwapToWaking(&sites[i]); err != nil {
			fmt.Printf("[WARN] idle-suspend waking vhost %s: %v\n", sites[i].Name, err)
		}
	}
	if len(sites) > 0 {
		idleReloadNginx()
	}
	// Flag first, so nothing watching sees the service stopped but not asleep
	// and reports it as stopped.
	for _, s := range stopped {
		_ = config.SetServiceIdleSuspended(s, true)
	}
	if err := idleStopService(name); err != nil {
		for _, s := range stopped {
			_ = config.SetServiceIdleSuspended(s, false)
		}
		return nil, err
	}
	now := time.Now()
	for _, s := range stopped {
		_ = config.SetServiceSleptAt(s, now)
	}
	return stopped, nil
}

// WakeServicesForIdle starts services idle-suspend put to sleep, waiting until
// each is ready, then gives back the real vhost to every site whose services
// are all up again. Safe to call for a service something else already started.
func WakeServicesForIdle(names []string) error {
	// In parallel: a request is held until the slowest service is ready, not
	// until all of them have been started one after another.
	errs := make([]error, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = idleEnsureService(name)
		}()
	}
	wg.Wait()
	var firstErr error
	for i, name := range names {
		if errs[i] != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("waking %s: %w", name, errs[i])
			}
			continue
		}
		_ = config.SetServiceIdleSuspended(name, false)
	}
	restoreSitesAfterServiceWake(names)
	return firstErr
}

// restoreSitesAfterServiceWake regenerates the vhost of every site using one of
// the woken services, unless the site still waits on another sleeping service
// or on its own sleeping dev server, whose resume restores it instead.
func restoreSitesAfterServiceWake(names []string) {
	seen := map[string]bool{}
	restored := false
	for _, name := range names {
		for _, site := range idleSitesUsing(name) {
			if seen[site.Name] {
				continue
			}
			seen[site.Name] = true
			if siteWaitsOnSleepingService(site.Name) {
				continue
			}
			if hostProxyVhostSwapApplies(site.IsHostProxy(), site.IdleSuspendedWorkers) {
				continue
			}
			s := site
			if err := idleRestoreVhost(&s); err != nil {
				fmt.Printf("[WARN] idle-resume vhost %s: %v\n", site.Name, err)
				continue
			}
			restored = true
		}
	}
	if restored {
		idleReloadNginx()
	}
}

// siteWaitsOnSleepingService reports whether any service the site uses is
// still held asleep by idle-suspend.
func siteWaitsOnSleepingService(siteName string) bool {
	for _, svc := range config.IdleSuspendedServices() {
		for _, s := range idleSitesUsing(svc) {
			if s.Name == siteName {
				return true
			}
		}
	}
	return false
}

// IdleServiceStateIsStale reports whether a service recorded asleep is in fact
// running, started by the CLI, the dashboard or a login. The engine then wakes
// it formally so the sites using it get their vhost back.
func IdleServiceStateIsStale(name string) bool {
	return idleServiceFlagged(name) && idleServiceUp(name)
}

// swapSiteToWaking points a site's vhost at the auto-refreshing waking page.
func swapSiteToWaking(site *config.Site) error {
	if err := writeWakingHTML(site); err != nil {
		return err
	}
	return nginx.GenerateWakingVhost(*site)
}

func serviceUnitUp(name string) bool {
	status, _ := podman.UnitStatus("lerd-" + name)
	return status == "active" || status == "activating"
}
