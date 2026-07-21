package ui

import (
	"encoding/json"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/eventbus"
	"github.com/geodro/lerd/internal/workerheal"
)

// healthWatchInterval is how often the watcher re-runs the detector while a
// UI tab is visible. healthWatchIdleInterval is the cadence with no tab open,
// where the only consumer is the failure push notification.
//
// The detector is not free on darwin: siteinfo.AllUnitStates shells out to
// `launchctl print` once per lerd-*.plist, so a 25-worker install pays 25
// forks per uncached tick. The visible cadence stays above the 3s unit-state
// cache TTL so back-to-back dashboard renders share one sweep, and the idle
// cadence keeps an unattended machine off the CPU without going silent.
const (
	healthWatchInterval     = 5 * time.Second
	healthWatchIdleInterval = 60 * time.Second
)

// workerHealthDeps is the injection surface for tickWorkerHealth so the
// detect-diff-publish logic can be tested without launchd or the event bus.
type workerHealthDeps struct {
	detect  func() ([]workerheal.UnhealthyWorker, error)
	visible func() bool
	notify  func([]workerheal.UnhealthyWorker)
	publish func()
}

// defaultWorkerHealthDeps wires the production detector, bus, and batcher.
func defaultWorkerHealthDeps() workerHealthDeps {
	return workerHealthDeps{
		detect:  workerheal.Detect,
		visible: func() bool { return visibleClients.Load() > 0 },
		notify:  queueWorkerFailureNotifications,
		publish: func() { eventbus.Default.Publish(eventbus.KindSites) },
	}
}

// chooseHealthInterval picks the cadence for the next tick. Detection cannot
// be gated on visibility outright: the push notification exists precisely for
// the user whose dashboard is closed, so a hidden watcher slows down rather
// than stopping.
func chooseHealthInterval(visible bool) time.Duration {
	if visible {
		return healthWatchInterval
	}
	return healthWatchIdleInterval
}

// lastHealthSig is the last unhealthy-set signature seen by the watcher.
// Stored as an unsafe pointer through atomic so the watcher and broker
// don't race when the watcher updates it after publishing.
var lastHealthSig atomic.Value // string

// lastUnhealthySet is the previous tick's unhealthy slice, kept so the
// watcher can compute the *new* failures (set difference) and fire a
// notification only for units that just transitioned into failure — not
// every tick that re-broadcasts the same long-standing failure.
var lastUnhealthySet atomic.Value // []workerheal.UnhealthyWorker

// healthWatcherInitialized gates first-tick notifications. The first run
// seeds lastUnhealthySet from whatever workers were already failed when
// lerd-ui came up, without firing — otherwise a launchd restart with N
// pre-existing failures would dispatch N notifications instantly.
var healthWatcherInitialized atomic.Bool

// runWorkerHealthWatcher closes the gap between systemd's internal state
// transitions (start-limit-hit, external `systemctl stop`, anything that
// happens without lerd-ui's involvement) and the dashboard banner.
//
// The cadence is re-chosen after every tick so a dashboard opening or closing
// takes effect on the next round rather than at process restart.
//
// The watcher does NOT run the heal itself; it only surfaces drift.
func runWorkerHealthWatcher() {
	// A fresh lerd-ui process means a running session (boot/autostart, update, or
	// a manual restart) — `lerd stop` never restarts lerd-ui. Clear any stale
	// stop marker so a stop-then-reboot-autostart doesn't leave detection
	// suppressed forever; an actual `lerd stop` re-sets it while this process
	// keeps running.
	_ = config.ClearStopped()
	seedHealthState()
	deps := defaultWorkerHealthDeps()
	for {
		time.Sleep(chooseHealthInterval(deps.visible()))
		tickWorkerHealth(deps)
	}
}

// tickWorkerHealth runs one detection and publishes on a changed unhealthy
// set. Each tick:
//
//  1. Read the unhealthy set from the cached detector.
//  2. Compare to the last seen signature. If unchanged, do nothing.
//  3. Queue notifications for units that just entered failure. This runs
//     whether or not a tab is open, so a closed PWA still gets the push.
//  4. If a tab is visible, publish KindSites so the snapshot path rebuilds
//     the unhealthy_workers JSON and the broker pushes it to every tab.
func tickWorkerHealth(d workerHealthDeps) {
	unhealthy, err := d.detect()
	if err != nil {
		return
	}
	sig := healthSignature(unhealthy)
	prev, _ := lastHealthSig.Load().(string)
	if sig == prev {
		return
	}
	lastHealthSig.Store(sig)
	d.notify(diffNewFailuresAndCommit(unhealthy))
	// Skip the eventbus publish when no tab is open; the snapshot rebuild
	// would just rebuild bytes nobody reads.
	if !d.visible() {
		return
	}
	d.publish()
}

// seedHealthState records the baseline at process start so a launchd
// restart with pre-existing failures doesn't fire N notifications on the
// first tick, AND a clean start still notifies when a failure later
// appears (the loop's sig-changed guard would otherwise leave the helper's
// initialized flag false until the first state change, swallowing it).
func seedHealthState() {
	unhealthy, err := workerheal.Detect()
	if err != nil {
		return
	}
	lastHealthSig.Store(healthSignature(unhealthy))
	lastUnhealthySet.Store(append([]workerheal.UnhealthyWorker(nil), unhealthy...))
	healthWatcherInitialized.Store(true)
}

// diffNewFailuresAndCommit returns workers in unhealthy that weren't
// present last tick, then commits the current set. First call returns
// nothing — it only seeds state — defensive belt-and-suspenders for
// callers that didn't invoke seedHealthState first.
func diffNewFailuresAndCommit(unhealthy []workerheal.UnhealthyWorker) []workerheal.UnhealthyWorker {
	prev, _ := lastUnhealthySet.Load().([]workerheal.UnhealthyWorker)
	var out []workerheal.UnhealthyWorker
	if healthWatcherInitialized.Load() {
		out = newWorkerFailures(prev, unhealthy)
	}
	lastUnhealthySet.Store(append([]workerheal.UnhealthyWorker(nil), unhealthy...))
	healthWatcherInitialized.Store(true)
	return out
}

// healthSignature renders a stable string for set-equality checks. Sorting
// keeps the comparison robust against map-iteration order.
func healthSignature(ws []workerheal.UnhealthyWorker) string {
	if len(ws) == 0 {
		return ""
	}
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = w.Unit + ":" + w.State
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// buildUnhealthyWorkersJSON serialises the current detector output. Errors
// degrade to an empty array so the dashboard never sees a malformed frame.
// Each entry is enriched with the last journal line so the dashboard can
// surface "why did this fail?" without a drill-down.
func buildUnhealthyWorkersJSON() []byte {
	out, err := workerheal.Detect()
	if err != nil || len(out) == 0 {
		return []byte("[]")
	}
	out = workerheal.Enrich(out)
	b, err := json.Marshal(out)
	if err != nil {
		return []byte("[]")
	}
	return b
}
