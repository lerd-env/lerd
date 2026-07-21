package watcher

import (
	"testing"
	"time"

	"github.com/geodro/lerd/internal/power"
)

func TestPowerWatchState_NoRestartWithoutAChange(t *testing.T) {
	s := &powerWatchState{last: power.Mains, cooldown: powerRestartCooldown}
	now := time.Now()

	if s.shouldRestart(power.Mains, now) {
		t.Error("steady mains must not restart anything")
	}
	if s.shouldRestart(power.Mains, now.Add(time.Hour)) {
		t.Error("still steady an hour later must not restart anything")
	}
}

func TestPowerWatchState_RestartsOnTransition(t *testing.T) {
	s := &powerWatchState{last: power.Mains, cooldown: powerRestartCooldown}
	now := time.Now()

	if !s.shouldRestart(power.Battery, now) {
		t.Fatal("unplugging must re-apply the cadence")
	}
	if s.shouldRestart(power.Battery, now.Add(time.Second)) {
		t.Error("no further restart while the state holds")
	}
}

// Unplugging and replugging in quick succession should not bounce every reload
// worker each time.
func TestPowerWatchState_CooldownSuppressesFlapping(t *testing.T) {
	s := &powerWatchState{last: power.Mains, cooldown: 5 * time.Minute}
	now := time.Now()

	if !s.shouldRestart(power.Battery, now) {
		t.Fatal("first transition should restart")
	}
	if s.shouldRestart(power.Mains, now.Add(10*time.Second)) {
		t.Error("a transition inside the cooldown must be suppressed")
	}
}

// A transition suppressed by the cooldown must be retried once the window
// passes: the workers are still running the old interval, and dropping the
// change would strand them there until the power source moved again.
func TestPowerWatchState_SuppressedTransitionIsRetried(t *testing.T) {
	s := &powerWatchState{last: power.Mains, cooldown: 5 * time.Minute}
	now := time.Now()

	s.shouldRestart(power.Battery, now)                     // restarts, starts the cooldown
	if s.shouldRestart(power.Mains, now.Add(time.Minute)) { // suppressed
		t.Fatal("expected suppression inside the cooldown")
	}
	if !s.shouldRestart(power.Mains, now.Add(6*time.Minute)) {
		t.Error("the suppressed transition must be retried after the cooldown")
	}
}

// A host where no watcher can poll has no cadence to maintain, so the loop must
// not start at all: it would probe the power state forever and log transitions
// that restart nothing.
func TestWatchPower_StandsDownWhereNothingPolls(t *testing.T) {
	prevHost, prevCurrent := hostCanPollFn, powerCurrentFn
	probes := 0
	hostCanPollFn = func() bool { return false }
	powerCurrentFn = func() power.State {
		probes++
		return power.Mains
	}
	t.Cleanup(func() { hostCanPollFn, powerCurrentFn = prevHost, prevCurrent })

	done := make(chan struct{})
	go func() {
		WatchPower(time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WatchPower kept ticking on a host that never polls")
	}
	if probes != 0 {
		t.Errorf("probed the power state %d times with nothing to maintain, want 0", probes)
	}
}

// Every state carries a distinct interval, so each transition is worth acting
// on rather than being collapsed.
func TestPowerWatchState_LowPowerIsItsOwnTransition(t *testing.T) {
	s := &powerWatchState{last: power.Battery, cooldown: 0}
	now := time.Now()

	if !s.shouldRestart(power.LowPower, now) {
		t.Error("battery to low power changes the interval and must re-apply")
	}
}
