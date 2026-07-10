package siteinfo

import "testing"

// A scheduled worker is Type=oneshot driven by a .timer: its .service is
// inactive between ticks, so liveness is the timer's state, not the service's.
func TestWorkerUnitLiveness(t *testing.T) {
	cases := []struct {
		name         string
		schedule     string
		serviceState string
		timerState   string
		wantRunning  bool
		wantFailing  bool
	}{
		{"daemon running", "", "active", "", true, false},
		{"daemon stopped", "", "inactive", "", false, false},
		{"daemon failed", "", "failed", "", false, true},
		{"daemon starting", "", "activating", "", true, false},

		// The case the UI got wrong: armed timer, idle service.
		{"scheduled armed between ticks", "minutely", "inactive", "active", true, false},
		{"scheduled firing right now", "minutely", "activating", "active", true, false},
		{"scheduled timer stopped", "minutely", "inactive", "inactive", false, false},
		// A failed last run still surfaces, even though the timer is armed.
		{"scheduled last run failed", "minutely", "failed", "active", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			running, failing := workerLiveness(tc.schedule, tc.serviceState, tc.timerState)
			if running != tc.wantRunning || failing != tc.wantFailing {
				t.Fatalf("running=%v failing=%v, want running=%v failing=%v",
					running, failing, tc.wantRunning, tc.wantFailing)
			}
		})
	}
}
