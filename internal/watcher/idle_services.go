package watcher

import (
	"fmt"
	"sort"
	"time"

	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
)

// svcKeyPrefix marks an idle key that belongs to a service rather than a site:
// traffic to its dashboard or its own domain. A site name never contains ":".
const svcKeyPrefix = "svc:"

func svcKey(name string) string { return svcKeyPrefix + name }

// The service operations the engine drives, vars so tests can stand in for
// containers and nginx.
var (
	runningServices = cli.RunningServicesForIdle
	serviceUsers    = cli.IdleServiceUsers
	suspendService  = cli.SuspendServiceForIdle
	wakeServices    = cli.WakeServicesForIdle
	serviceStale    = cli.IdleServiceStateIsStale
	serviceStartRaw = serviceops.WakeService
	serviceStopRaw  = serviceops.StopWithDependents
	servicePinned   = config.ServiceIsPinned
	serviceAsleep   = config.ServiceIsIdleSuspended
)

// tickServices puts a service to sleep once every key that keeps it awake (the
// sites using it, their worktrees, its own dashboard and the dashboards of the
// services built on it) has been idle past the timeout, and wakes it again when
// one of them is active. With the feature off it only wakes what it slept.
func (e *idleEngine) tickServices(on bool, timeout time.Duration, now time.Time) {
	e.syncSleeping()
	if !on {
		if names := e.sleepingNames(); len(names) > 0 {
			e.wakeServicesAsync(names)
		}
		return
	}
	candidates := map[string]bool{}
	for _, name := range runningServices() {
		candidates[name] = true
	}
	for _, name := range e.sleepingNames() {
		candidates[name] = true
	}
	var wake []string
	for name := range candidates {
		keys, exempt := e.serviceKeys(name)
		e.mu.Lock()
		e.svcKeys[name] = keys
		asleep := e.sleeping[name]
		// Mid-suspend it is already flagged but still up; that is not stale.
		busy := e.inFlight[svcKey(name)]
		e.mu.Unlock()
		if busy {
			continue
		}
		if asleep {
			// A service asleep but running was started behind the engine's back
			// (the CLI, a dashboard, a login): wake it formally so its sites get
			// their vhost back.
			if exempt || serviceStale(name) || e.minIdle(keys, now) < timeout {
				wake = append(wake, name)
			}
			continue
		}
		if !exempt && e.minIdle(keys, now) >= timeout {
			e.suspendServiceAsync(name)
		}
	}
	if len(wake) > 0 {
		sort.Strings(wake)
		e.wakeServicesAsync(wake)
	}
}

// serviceKeys returns the activity keys that keep a service awake, and whether
// it is exempt because it is pinned or a site using it never idles.
func (e *idleEngine) serviceKeys(name string) (keys []string, exempt bool) {
	sites, dependents := serviceUsers(name)
	keys = []string{svcKey(name)}
	for _, dep := range dependents {
		keys = append(keys, svcKey(dep))
	}
	exempt = servicePinned(name)
	for i := range sites {
		s := &sites[i]
		if neverIdles(s) {
			exempt = true
		}
		keys = append(keys, s.Name)
		for _, wt := range wtIndex.forSite(s.Name) {
			keys = append(keys, wtKey(s.Name, wt.Base))
		}
	}
	return keys, exempt
}

// minIdle is how long the most recently active key has been idle. A key never
// seen before starts its grace window now, as a new worktree does.
func (e *idleEngine) minIdle(keys []string, now time.Time) time.Duration {
	least := time.Duration(1<<63 - 1)
	for _, k := range keys {
		d, ok := e.tracker.IdleFor(k, now)
		if !ok {
			e.tracker.TouchSite(k, now)
			d = 0
		}
		if d < least {
			least = d
		}
	}
	return least
}

// sleepingFor returns the sleeping services a site or worktree key keeps awake.
func (e *idleEngine) sleepingFor(key string) []string {
	site, _, _ := splitWtKey(key)
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for name, asleep := range e.sleeping {
		if !asleep {
			continue
		}
		for _, k := range e.svcKeys[name] {
			if k == key || k == site {
				out = append(out, name)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// syncSleeping reloads the sleeping set from disk, the source of truth: a user
// stopping a sleeping service clears its flag, and the engine must then leave
// it down rather than wake it.
func (e *idleEngine) syncSleeping() {
	names := config.IdleSuspendedServices()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sleeping = make(map[string]bool, len(names))
	for _, n := range names {
		e.sleeping[n] = true
	}
}

func (e *idleEngine) sleepingNames() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for name, asleep := range e.sleeping {
		if asleep {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func (e *idleEngine) suspendServiceAsync(name string) {
	key := svcKey(name)
	e.mu.Lock()
	if e.inFlight[key] {
		e.mu.Unlock()
		return
	}
	e.inFlight[key] = true
	e.mu.Unlock()

	e.spawn("suspend-svc", func() {
		defer e.clearInFlight(key)
		e.svcMu.Lock()
		defer e.svcMu.Unlock()
		stopped, err := suspendService(name)
		if err != nil {
			fmt.Printf("[WARN] idle-suspend service %s: %v\n", name, err)
			return
		}
		e.mu.Lock()
		for _, s := range stopped {
			e.sleeping[s] = true
		}
		e.mu.Unlock()
		fmt.Printf("[idle] suspended services: %v\n", stopped)
		publishSitesChanged()
	})
}

// wakeServicesAsync wakes the named services in the background. A no-op for
// names that are not asleep, which is the common case on every request.
func (e *idleEngine) wakeServicesAsync(names []string) {
	if e == nil || len(e.asleepOf(names)) == 0 {
		return
	}
	e.spawn("wake-svc", func() { e.wakeServicesNow(names) })
}

// wakeServicesNow wakes the named services that are still asleep and returns
// once they are ready, so a caller can start the workers that need them.
func (e *idleEngine) wakeServicesNow(names []string) {
	if e == nil {
		return
	}
	e.svcMu.Lock()
	defer e.svcMu.Unlock()
	names = e.asleepOf(names) // an earlier wake queued on svcMu may have done it
	if len(names) == 0 {
		return
	}
	if err := wakeServices(names); err != nil {
		fmt.Printf("[WARN] idle-resume services: %v\n", err)
	}
	// A wake is use: without this a service started from outside (a dashboard,
	// the CLI) is reconciled awake and put straight back to sleep next tick.
	now := time.Now()
	for _, n := range names {
		e.tracker.TouchSite(svcKey(n), now)
	}
	e.mu.Lock()
	for _, n := range names {
		e.sleeping[n] = serviceAsleep(n)
	}
	e.mu.Unlock()
	fmt.Printf("[idle] resumed services: %v\n", names)
	publishSitesChanged()
}

func (e *idleEngine) asleepOf(names []string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, n := range names {
		if e.sleeping[n] {
			out = append(out, n)
		}
	}
	return out
}

// withServiceBriefly starts a sleeping service just long enough to run fn and
// stops it again, leaving it asleep: its flag, its sites' waking vhosts and its
// countdown are untouched. The tick skips it meanwhile, and a request that
// wakes it for real queues behind svcMu and wakes it properly after.
func (e *idleEngine) withServiceBriefly(name string, fn func() error) error {
	if e == nil {
		return fmt.Errorf("idle-suspend is not running")
	}
	key := svcKey(name)
	e.mu.Lock()
	if e.inFlight[key] {
		e.mu.Unlock()
		return fmt.Errorf("%s is busy", name)
	}
	e.inFlight[key] = true
	e.mu.Unlock()
	defer e.clearInFlight(key)

	e.svcMu.Lock()
	defer e.svcMu.Unlock()
	if err := serviceStartRaw(name); err != nil {
		return err
	}
	fnErr := fn()
	if err := serviceStopRaw(name); err != nil && fnErr == nil {
		return err
	}
	return fnErr
}
