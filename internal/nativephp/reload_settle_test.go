package nativephp

import (
	"testing"
	"time"
)

// launchctl kickstart returns as soon as launchd has taken the job, not when
// php-fpm holds the socket again, so a reload that returned there handed the
// very next request a 502. Measured ten toggles at a time, every one of them
// failed. The container runtime solves the same problem by probing the pool;
// this is that wait for the host one.
func TestWaitAcceptingReturnsOncePoolAnswers(t *testing.T) {
	calls := 0
	accepting := func() bool {
		calls++
		return calls >= 3
	}
	var slept time.Duration
	if !waitAccepting(accepting, time.Second, func(d time.Duration) { slept += d }) {
		t.Fatal("a pool that comes up must be reported as accepting")
	}
	if calls != 3 {
		t.Errorf("probed %d times, want 3", calls)
	}
	if slept == 0 {
		t.Error("expected it to wait between probes rather than spin")
	}
}

// A pool that never answers must not hang the command forever.
func TestWaitAcceptingGivesUpOnItsBudget(t *testing.T) {
	var slept time.Duration
	if waitAccepting(func() bool { return false }, 50*time.Millisecond, func(d time.Duration) { slept += d }) {
		t.Error("a pool that never answers must not report as accepting")
	}
	if slept > time.Second {
		t.Errorf("waited %s, far past the budget", slept)
	}
}

// One that is already accepting costs no wait at all.
func TestWaitAcceptingIsImmediateWhenUp(t *testing.T) {
	var slept time.Duration
	if !waitAccepting(func() bool { return true }, time.Second, func(d time.Duration) { slept += d }) {
		t.Fatal("an accepting pool must report immediately")
	}
	if slept != 0 {
		t.Errorf("waited %s for a pool that was already up", slept)
	}
}
