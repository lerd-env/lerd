package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/plugins"
)

func TestScheduleRow_SaysWhenItRanAndRunsNext(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.Local)
	cases := []struct {
		name string
		in   plugins.ScheduleStatus
		want []string
	}{
		{"never ran", plugins.ScheduleStatus{Name: "sweep", Every: "1h"},
			[]string{"sweep", "1h", "never", "within a minute", "waiting"}},
		{"ran fine", plugins.ScheduleStatus{Name: "sweep", Every: "1h", LastRun: now.Add(-20 * time.Minute), NextRun: now.Add(40 * time.Minute)},
			[]string{"sweep", "1h", "11:40", "12:40", "ok"}},
		{"failed", plugins.ScheduleStatus{Name: "sweep", Every: "1h", LastRun: now.Add(-time.Hour), NextRun: now, LastError: "boom\nexit status 3"},
			[]string{"sweep", "1h", "11:00", "12:00", "failed: boom"}},
		{"overdue", plugins.ScheduleStatus{Name: "sweep", Every: "1h", LastRun: now.Add(-3 * time.Hour), NextRun: now.Add(-2 * time.Hour)},
			[]string{"sweep", "1h", "09:00", "within a minute", "ok"}},
		{"running", plugins.ScheduleStatus{Name: "sweep", Every: "1h", LastRun: now.Add(-time.Hour), NextRun: now, Running: true},
			[]string{"sweep", "1h", "11:00", "12:00", "running"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := scheduleRow(c.in, now)
			if strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Errorf("row = %q, want %q", got, c.want)
			}
		})
	}
}

// A run from yesterday shows its date, so it is never read as today's.
func TestScheduleRow_OlderRunsShowTheDate(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.Local)
	got := scheduleRow(plugins.ScheduleStatus{Name: "nightly", Every: "24h", LastRun: now.Add(-30 * time.Hour), NextRun: now.Add(-6 * time.Hour)}, now)
	if got[2] != "Nov 13 06:00" {
		t.Errorf("last run = %q, want the date with it", got[2])
	}
}
