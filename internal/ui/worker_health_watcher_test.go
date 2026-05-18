package ui

import (
	"testing"

	"github.com/geodro/lerd/internal/workerheal"
)

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
