package podman

import (
	"errors"
	"testing"
)

// A test that drives the unit lifecycle without installing a stub used to fall
// through to the real user bus and start, stop and enable units on the machine
// running the suite. That is how a crash-looping lerd-queue unit ended up on a
// developer's machine. It must refuse instead.
func TestUnitLifecycle_refusesRealSystemdUnderTest(t *testing.T) {
	prev := UnitLifecycle
	UnitLifecycle = nil
	t.Cleanup(func() { UnitLifecycle = prev })

	for _, tc := range []struct {
		name string
		call func(string) error
	}{
		{"start", StartUnit},
		{"stop", StopUnit},
		{"restart", RestartUnit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call("lerd-queue-myapp"); !errors.Is(err, errNoRealSystemd) {
				t.Errorf("%s reached the real systemd from a test (err = %v)", tc.name, err)
			}
		})
	}
}

// With a stub installed the lifecycle still works, so the refusal only covers the
// unstubbed path and doesn't disarm the tests that do drive it.
func TestUnitLifecycle_stubStillDrivesTheLifecycle(t *testing.T) {
	stub := &recordingLifecycle{}
	prev := UnitLifecycle
	UnitLifecycle = stub
	t.Cleanup(func() { UnitLifecycle = prev })

	if err := StartUnit("lerd-queue-myapp"); err != nil {
		t.Fatalf("start with a stub: %v", err)
	}
	if stub.started != "lerd-queue-myapp" {
		t.Errorf("stub should have seen the start, got %q", stub.started)
	}
}

type recordingLifecycle struct{ started string }

func (r *recordingLifecycle) Start(name string) error           { r.started = name; return nil }
func (r *recordingLifecycle) Stop(string) error                 { return nil }
func (r *recordingLifecycle) Restart(string) error              { return nil }
func (r *recordingLifecycle) UnitStatus(string) (string, error) { return "inactive", nil }
func (r *recordingLifecycle) AllUnitStates() map[string]string  { return nil }
