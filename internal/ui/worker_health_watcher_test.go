package ui

import (
	"testing"
	"time"

	"github.com/geodro/lerd/internal/workerheal"
)

// testHealthDeps returns a deps set wired to counters plus pointers to them,
// so each test can assert what the tick actually reached for.
func testHealthDeps(visible bool, unhealthy []workerheal.UnhealthyWorker) (workerHealthDeps, *int, *int, *int) {
	detects, notifies, publishes := 0, 0, 0
	d := workerHealthDeps{
		detect: func() ([]workerheal.UnhealthyWorker, error) {
			detects++
			return unhealthy, nil
		},
		visible: func() bool { return visible },
		notify:  func([]workerheal.UnhealthyWorker) { notifies++ },
		publish: func() { publishes++ },
	}
	return d, &detects, &notifies, &publishes
}

// A closed dashboard must still detect and notify: the push notification is
// the whole point of running the watcher when nobody is looking. Gating
// detection on visibility would silence failures for closed-PWA users.
func TestTickWorkerHealth_HiddenStillDetectsAndNotifies(t *testing.T) {
	t.Cleanup(resetHealthState)
	resetHealthState()
	healthWatcherInitialized.Store(true)

	d, detects, notifies, publishes := testHealthDeps(false,
		[]workerheal.UnhealthyWorker{{Unit: "lerd-horizon-a", State: "failed"}})
	tickWorkerHealth(d)

	if *detects != 1 {
		t.Errorf("hidden tick must still detect: got %d calls", *detects)
	}
	if *notifies != 1 {
		t.Errorf("hidden tick must still notify: got %d calls", *notifies)
	}
	if *publishes != 0 {
		t.Errorf("hidden tick must not publish to the bus: got %d calls", *publishes)
	}
}

// With a tab open the same transition also publishes, so the banner updates.
func TestTickWorkerHealth_VisiblePublishes(t *testing.T) {
	t.Cleanup(resetHealthState)
	resetHealthState()
	healthWatcherInitialized.Store(true)

	d, _, _, publishes := testHealthDeps(true,
		[]workerheal.UnhealthyWorker{{Unit: "lerd-horizon-a", State: "failed"}})
	tickWorkerHealth(d)

	if *publishes != 1 {
		t.Errorf("visible tick must publish: got %d calls", *publishes)
	}
}

// An unchanged signature costs one detect and nothing else.
func TestTickWorkerHealth_UnchangedSignatureDoesNotPublish(t *testing.T) {
	t.Cleanup(resetHealthState)
	resetHealthState()
	healthWatcherInitialized.Store(true)

	unhealthy := []workerheal.UnhealthyWorker{{Unit: "lerd-horizon-a", State: "failed"}}
	d, _, _, publishes := testHealthDeps(true, unhealthy)
	tickWorkerHealth(d)
	tickWorkerHealth(d)

	if *publishes != 1 {
		t.Errorf("steady state must publish once, not per tick: got %d", *publishes)
	}
}

// The idle cadence is what keeps an unattended install off the CPU. On
// darwin every detect is one launchctl fork per worker plist, so a hidden
// dashboard must poll far less often than a visible one.
func TestChooseHealthInterval(t *testing.T) {
	if got := chooseHealthInterval(true); got != healthWatchInterval {
		t.Errorf("visible interval = %v, want %v", got, healthWatchInterval)
	}
	if got := chooseHealthInterval(false); got != healthWatchIdleInterval {
		t.Errorf("hidden interval = %v, want %v", got, healthWatchIdleInterval)
	}
	if healthWatchIdleInterval <= healthWatchInterval {
		t.Fatal("idle cadence must be slower than the visible one")
	}
}

// The unit-state cache exists to absorb repeat detects. A tick faster than
// the cache TTL misses it every single time, which is the bug behind the
// idle fork storm: the throttle never once fires for this caller.
func TestHealthTickOutlivesUnitCacheTTL(t *testing.T) {
	const darwinUnitStatesTTL = 3 * time.Second
	if healthWatchInterval < darwinUnitStatesTTL {
		t.Errorf("tick %v is shorter than the %v unit-state cache TTL, so every tick is a cache miss",
			healthWatchInterval, darwinUnitStatesTTL)
	}
}

func resetHealthState() {
	lastUnhealthySet.Store([]workerheal.UnhealthyWorker(nil))
	lastHealthSig.Store("")
	healthWatcherInitialized.Store(false)
}

// Self-review caught a bug in the original fix: on a clean start
// (no failures), the loop's sig-unchanged guard skipped the body, so the
// first real failure landed at a still-uninitialised diff helper and was
// swallowed. seedHealthState() before the loop fixes that — verify here.
func TestCleanStartThenFirstFailureFires(t *testing.T) {
	t.Cleanup(resetHealthState)
	resetHealthState()

	// Simulate seedHealthState() with an empty baseline (clean start).
	lastHealthSig.Store(healthSignature(nil))
	lastUnhealthySet.Store([]workerheal.UnhealthyWorker(nil))
	healthWatcherInitialized.Store(true)

	// Now a worker fails for the first time.
	got := diffNewFailuresAndCommit([]workerheal.UnhealthyWorker{
		{Unit: "lerd-horizon-a", State: "failed"},
	})
	if len(got) != 1 || got[0].Unit != "lerd-horizon-a" {
		t.Errorf("first failure after clean start must dispatch: got %+v", got)
	}
}

// First tick after process start seeds lastUnhealthySet without firing.
// Otherwise a launchd restart with N already-failed workers would dispatch
// N notifications instantly.
func TestDiffNewFailuresAndCommit_FirstTickSilent(t *testing.T) {
	t.Cleanup(resetHealthState)
	resetHealthState()

	pre := []workerheal.UnhealthyWorker{
		{Unit: "lerd-horizon-a", State: "failed"},
		{Unit: "lerd-horizon-b", State: "failed"},
	}
	got := diffNewFailuresAndCommit(pre)
	if len(got) != 0 {
		t.Errorf("first tick must return no failures (got %d), to avoid startup storm", len(got))
	}
}

// After the seeding tick, genuinely-new failures are reported and
// already-failed units are ignored.
func TestDiffNewFailuresAndCommit_SecondTickReportsNewOnly(t *testing.T) {
	t.Cleanup(resetHealthState)
	resetHealthState()

	pre := []workerheal.UnhealthyWorker{{Unit: "lerd-horizon-a", State: "failed"}}
	_ = diffNewFailuresAndCommit(pre)

	next := []workerheal.UnhealthyWorker{
		{Unit: "lerd-horizon-a", State: "failed"},
		{Unit: "lerd-horizon-b", State: "failed"},
	}
	got := diffNewFailuresAndCommit(next)
	if len(got) != 1 || got[0].Unit != "lerd-horizon-b" {
		t.Errorf("second tick should report only horizon-b: %+v", got)
	}
}
