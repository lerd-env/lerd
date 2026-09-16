package systemd

import (
	"testing"
	"time"
)

// systemd queues a start or a restart behind a stop that is already in flight,
// so those ops wait out the stop before their own job even begins. Only the
// stop read the unit's declared window, which left a start on a slow service
// with a fixed 30s budget it could not possibly meet:
//
//	✗ could not start mysql: start lerd-mysql timed out after 30s
//
// while the unit log showed mysqld ready for connections a second after the
// start actually started. mysql declares TimeoutStopSec=75, and takes about
// that long to come down, so every db command issued during a restart failed.
func TestUnitOpWaitCoversAQueuedStopForEveryOp(t *testing.T) {
	const declared = 75 * time.Second
	const wantSlow = 85 * time.Second

	for _, op := range []string{"stop", "start", "restart"} {
		t.Run(op+" on a unit with a long stop window", func(t *testing.T) {
			if got := unitOpJobWait(op, declared); got != wantSlow {
				t.Errorf("unitOpJobWait(%q, %s) = %s, want %s", op, declared, got, wantSlow)
			}
		})
	}
}

// A unit that declares nothing, or a short window, keeps the historic floor for
// every op, so nothing waits longer than it used to on a fast service.
func TestUnitOpWaitKeepsTheFloorForFastUnits(t *testing.T) {
	for _, op := range []string{"stop", "start", "restart"} {
		for _, declared := range []time.Duration{0, -1, 5 * time.Second, 20 * time.Second} {
			if got := unitOpJobWait(op, declared); got != jobWaitFloor {
				t.Errorf("unitOpJobWait(%q, %s) = %s, want the floor %s", op, declared, got, jobWaitFloor)
			}
		}
	}
}
