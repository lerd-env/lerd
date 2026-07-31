package ui

import (
	"log"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/eventbus"
)

// snapshotTTL bounds how long a cached snapshot is reused before the next
// request triggers a rebuild. Mutations that change state call
// eventbus.Default.Publish, which invalidates the relevant kind so the next
// read recomputes immediately.
//
// Two cadences: the short one when a real UI tab is open (the dashboard
// expects near-realtime data), the long one when only the tray is polling.
// The tray shows slow-changing state (nginx running, dns ok, php list); five
// minutes of staleness there is fine because explicit mutations invalidate
// via AfterUnitChange and are reflected on the next tray poll regardless.
const (
	snapshotTTLActive = 15 * time.Second
	snapshotTTLIdle   = 5 * time.Minute
)

func currentSnapshotTTL() time.Duration {
	if visibleClients.Load() > 0 {
		return snapshotTTLActive
	}
	return snapshotTTLIdle
}

// snapshotSlot caches one kind's JSON bytes behind its own build mutex, so
// that when podman inspect is slow the callers queue behind a single in-flight
// rebuild rather than each spawning their own batch of subprocesses.
//
// get returns nil only when there is nothing truthful to say: no cached value
// and a build that failed. Callers must treat nil as "unavailable" rather than
// as an empty result, because presenting it as data is how a cold cache turned
// into a dashboard with no sites on it.
type snapshotSlot struct {
	mu    sync.Mutex
	build sync.Mutex

	data []byte
	at   time.Time

	fn func() ([]byte, error)
}

func (s *snapshotSlot) get() []byte {
	s.mu.Lock()
	if s.data != nil && time.Since(s.at) < currentSnapshotTTL() {
		b := s.data
		s.mu.Unlock()
		return b
	}
	stale := s.data
	s.mu.Unlock()

	// A caller holding a usable value never waits on a slow rebuild. A caller
	// with nothing has to, or it answers with the cold cache's nil and the
	// dashboard renders that as an empty list.
	if stale != nil {
		if !s.build.TryLock() {
			return stale
		}
	} else {
		s.build.Lock()
	}
	defer s.build.Unlock()

	s.mu.Lock()
	if s.data != nil && time.Since(s.at) < currentSnapshotTTL() {
		b := s.data
		s.mu.Unlock()
		return b
	}
	s.mu.Unlock()

	b, err := s.fn()
	if err != nil {
		// Storing this would serve one transient failure as the truth for a
		// whole TTL, up to five minutes when no tab is counted visible.
		log.Printf("[snapshot] rebuild failed, keeping previous value: %v", err)
		return stale
	}

	s.mu.Lock()
	s.data = b
	s.at = time.Now()
	s.mu.Unlock()
	return b
}

// invalidate drops the cached value's freshness so the next read rebuilds.
func (s *snapshotSlot) invalidate() {
	s.mu.Lock()
	s.at = time.Time{}
	s.mu.Unlock()
}

// snapshotCache holds the cached JSON of the last /api/sites, /api/services,
// and /api/status responses. Handlers read from here instead of rebuilding
// from scratch on every poll; /api/ws broadcasts the same bytes to every
// connected browser.
type snapshotCache struct {
	sites, services, status snapshotSlot

	// Worker health shadows the sites cycle: it is derived from the same
	// batched unit-state cache and invalidated alongside KindSites, so callers
	// don't pay any extra subprocess cost.
	unhealthy snapshotSlot
}

var snapshots = &snapshotCache{
	sites:     snapshotSlot{fn: buildSitesJSON},
	services:  snapshotSlot{fn: buildServicesJSON},
	status:    snapshotSlot{fn: buildStatusJSON},
	unhealthy: snapshotSlot{fn: buildUnhealthyWorkersJSON},
}

// Sites returns cached /api/sites JSON, rebuilding if stale. nil means no
// snapshot could be produced.
func (c *snapshotCache) Sites() []byte { return c.sites.get() }

// Services returns cached /api/services JSON, rebuilding if stale.
func (c *snapshotCache) Services() []byte { return c.services.get() }

// Status returns cached /api/status JSON, rebuilding if stale.
func (c *snapshotCache) Status() []byte { return c.status.get() }

// UnhealthyWorkers returns cached worker-health JSON, rebuilding if stale.
// Pinned to the KindSites lifecycle: same source cache, same invalidation
// signal, no extra polling.
func (c *snapshotCache) UnhealthyWorkers() []byte { return c.unhealthy.get() }

// Invalidate drops the cached bytes for one kind so the next read rebuilds.
func (c *snapshotCache) Invalidate(kind string) {
	switch kind {
	case eventbus.KindSites:
		c.sites.invalidate()
		c.unhealthy.invalidate()
	case eventbus.KindServices:
		c.services.invalidate()
	case eventbus.KindStatus:
		c.status.invalidate()
	}
}

// InvalidateAll drops all cached snapshots.
func (c *snapshotCache) InvalidateAll() {
	c.sites.invalidate()
	c.services.invalidate()
	c.status.invalidate()
	c.unhealthy.invalidate()
}
