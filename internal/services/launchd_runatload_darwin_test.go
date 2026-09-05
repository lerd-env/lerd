package services

import (
	"errors"
	"testing"
)

// bootstrap with RunAtLoad=true is supposed to start the job, but launchd does
// not always oblige: after an abrupt teardown the job loads and sits at "not
// running" while lerd reports success, which is how every worker ended up
// failed with nothing wrong in its own log.
func TestRunAtLoadIsVerifiedAndKicked(t *testing.T) {
	var kicked []string
	running := false

	err := ensureStarted("lerd-horizon-app",
		func(string) bool { return running },
		func(label string) error { kicked = append(kicked, label); running = true; return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kicked) != 1 {
		t.Errorf("a loaded-but-idle job must be kicked, got %v", kicked)
	}
}

// A job bootstrap really did start must not be kicked: that would restart a
// healthy worker for nothing.
func TestRunningJobIsLeftAlone(t *testing.T) {
	var kicked []string
	err := ensureStarted("lerd-horizon-app",
		func(string) bool { return true },
		func(label string) error { kicked = append(kicked, label); return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kicked) != 0 {
		t.Errorf("a running job must be left alone, got %v", kicked)
	}
}

// A kick that fails surfaces, rather than reporting a start that never happened.
func TestKickFailureSurfaces(t *testing.T) {
	err := ensureStarted("lerd-horizon-app",
		func(string) bool { return false },
		func(string) error { return errors.New("boom") })
	if err == nil {
		t.Error("a failed kick must be reported, not swallowed")
	}
}
