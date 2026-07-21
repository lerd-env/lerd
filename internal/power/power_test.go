package power

import "testing"

// Callers arrive in bursts (one worker unit per worker per site), and on macOS
// each probe is a subprocess, so repeat calls must collapse into one.
func TestCurrent_CachesWithinTTL(t *testing.T) {
	t.Cleanup(reset)
	reset()

	probes := 0
	prev := probeFn
	probeFn = func() State {
		probes++
		return Battery
	}
	t.Cleanup(func() { probeFn = prev })

	for i := 0; i < 5; i++ {
		if Current() != Battery {
			t.Fatal("expected the stubbed probe's answer")
		}
	}
	if probes != 1 {
		t.Errorf("probed %d times for five calls inside the TTL, want 1", probes)
	}
}

// An undeterminable power source must read as mains, so a desktop or a host
// without sysfs keeps the normal cadence instead of being treated as a laptop.
func TestCurrent_DefaultsToMains(t *testing.T) {
	t.Cleanup(reset)
	reset()

	prev := probeFn
	probeFn = func() State { return Mains }
	t.Cleanup(func() { probeFn = prev })

	if got := Current(); got != Mains {
		t.Errorf("unknown power source = %v, want mains", got)
	}
}

func TestStateString(t *testing.T) {
	for _, tc := range []struct {
		s    State
		want string
	}{
		{Mains, "mains"},
		{Battery, "battery"},
		{LowPower, "low-power"},
	} {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("State(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}
