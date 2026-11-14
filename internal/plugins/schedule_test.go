package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// schedulePlugin installs a plugin with one schedule, every 10m, whose bin/run
// runs script.
func schedulePlugin(t *testing.T, name, script string) {
	t.Helper()
	dir := writePlugin(t, Dir(), name, "api_version: 1\nname: "+name+"\nschedules:\n  - {name: sweep, every: 10m, run: bin/run}\n")
	if err := os.WriteFile(filepath.Join(dir, "bin", "run"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestReadManifest_ScheduleRefusals(t *testing.T) {
	cases := []struct{ name, schedules, want string }{
		{"no interval", "  - {name: x, run: bin/run}\n", "every"},
		{"interval not a duration", "  - {name: x, every: soon, run: bin/run}\n", `"soon"`},
		{"interval too short", "  - {name: x, every: 30s, run: bin/run}\n", "30s"},
		{"bad name", "  - {name: X, every: 1h, run: bin/run}\n", `"X"`},
		{"twice", "  - {name: x, every: 1h, run: bin/run}\n  - {name: x, every: 2h, run: bin/run}\n", `"x"`},
		{"missing run", "  - {name: x, every: 1h, run: bin/nope}\n", "bin/nope"},
		{"timeout past the limit", "  - {name: x, every: 1h, run: bin/run, timeout: 2h}\n", "2h"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := writePlugin(t, t.TempDir(), "p", "api_version: 1\nname: p\nschedules:\n"+c.schedules)
			_, err := ReadPlugin(dir)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to name %s", err, c.want)
			}
		})
	}
}

// Asking to run on a schedule is a permission like notifications: pending
// until answered, kept across updates, forgotten on removal.
func TestScheduleState_FollowsTheUsersAnswer(t *testing.T) {
	isolate(t)
	state := func() string {
		p, err := Find("cron")
		if err != nil {
			t.Fatal(err)
		}
		return PermissionState(p, PermissionSchedule)
	}
	writePlugin(t, Dir(), "cron", "api_version: 1\nname: cron\n")
	if got := state(); got != PermissionNone {
		t.Fatalf("without schedules: %q, want none", got)
	}
	if err := SetPermission("cron", PermissionSchedule, true); err == nil {
		t.Fatal("a plugin without schedules cannot be allowed to run on one")
	}

	schedulePlugin(t, "cron", "#!/bin/sh\n")
	if got := state(); got != PermissionPending {
		t.Fatalf("after an update adds a schedule: %q, want pending", got)
	}
	if err := SetPermission("cron", PermissionSchedule, true); err != nil {
		t.Fatal(err)
	}
	if got := state(); got != PermissionAllowed {
		t.Fatalf("after allowing: %q", got)
	}
	if err := Remove("cron"); err != nil {
		t.Fatal(err)
	}
	schedulePlugin(t, "cron", "#!/bin/sh\n")
	if got := state(); got != PermissionPending {
		t.Fatalf("after a reinstall: %q, want pending again", got)
	}
}

func TestRunDue_OnlyRunsAllowedSchedulesWhenDue(t *testing.T) {
	isolate(t)
	schedulePlugin(t, "cron", "#!/bin/sh\necho \"$LERD_SURFACE|$LERD_SCHEDULE\" >> \"$LERD_PLUGIN_DATA/runs\"\n")
	runs := func() []string {
		data, _ := os.ReadFile(filepath.Join(DataDirFor("cron"), "runs"))
		return strings.Fields(string(data))
	}
	start := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

	RunDue(start)()
	if got := runs(); len(got) != 0 {
		t.Fatalf("ran before the user allowed it: %v", got)
	}

	if err := SetPermission("cron", PermissionSchedule, true); err != nil {
		t.Fatal(err)
	}
	RunDue(start)()
	if got := runs(); len(got) != 1 || got[0] != "schedule|sweep" {
		t.Fatalf("first allowed tick: runs = %v, want one schedule|sweep", got)
	}
	RunDue(start.Add(9 * time.Minute))()
	if got := runs(); len(got) != 1 {
		t.Fatalf("ran again before its interval: %v", got)
	}
	RunDue(start.Add(10 * time.Minute))()
	if got := runs(); len(got) != 2 {
		t.Fatalf("did not run once its interval passed: %v", got)
	}

	if err := SetPermission("cron", PermissionSchedule, false); err != nil {
		t.Fatal(err)
	}
	RunDue(start.Add(time.Hour))()
	if got := runs(); len(got) != 2 {
		t.Fatalf("ran after the user denied it: %v", got)
	}
}

// A run still going when the next tick comes is left alone, never doubled.
func TestRunDue_SkipsARunStillGoing(t *testing.T) {
	isolate(t)
	schedulePlugin(t, "slow", "#!/bin/sh\necho x >> \"$LERD_PLUGIN_DATA/runs\"\nwhile [ ! -e \"$LERD_PLUGIN_DATA/go\" ]; do sleep 0.05; done\n")
	if err := SetPermission("slow", PermissionSchedule, true); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	first := RunDue(start)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(DataDirFor("slow"), "runs")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first run never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Recorded as it starts, so a restart of lerd-ui mid-run does not start it again.
	if got := readRuns()["slow"]["sweep"].LastRun; !got.Equal(start) {
		t.Errorf("last run while running = %v, want the start %v", got, start)
	}
	RunDue(start.Add(time.Hour))()
	_ = os.WriteFile(filepath.Join(DataDirFor("slow"), "go"), nil, 0o644)
	first()

	data, _ := os.ReadFile(filepath.Join(DataDirFor("slow"), "runs"))
	if n := len(strings.Fields(string(data))); n != 1 {
		t.Fatalf("runs = %d, want the second tick to skip the running one", n)
	}
}

func TestRunDue_RecordsAFailureAndShowsIt(t *testing.T) {
	isolate(t)
	schedulePlugin(t, "broken", "#!/bin/sh\necho boom >&2\nexit 3\n")
	if err := SetPermission("broken", PermissionSchedule, true); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	RunDue(at)()

	p, err := Find("broken")
	if err != nil {
		t.Fatal(err)
	}
	got := ScheduleStatuses(p)
	if len(got) != 1 {
		t.Fatalf("statuses = %+v", got)
	}
	s := got[0]
	if s.Name != "sweep" || s.Every != "10m" || !s.LastRun.Equal(at) || !s.NextRun.Equal(at.Add(10*time.Minute)) {
		t.Errorf("status = %+v", s)
	}
	if !strings.Contains(s.LastError, "boom") {
		t.Errorf("last error = %q, want the script's stderr", s.LastError)
	}
	log, _ := os.ReadFile(filepath.Join(DataDirFor("broken"), ScheduleLog))
	if !strings.Contains(string(log), "boom") {
		t.Errorf("schedule.log = %q, want the output", log)
	}
}

// lerd stop pauses schedules, as it stops lerd's own scheduled workers, and
// lerd start brings them back.
func TestRunDue_PausedWhileLerdIsStopped(t *testing.T) {
	isolate(t)
	schedulePlugin(t, "cron", "#!/bin/sh\ntouch \"$LERD_PLUGIN_DATA/ran\"\n")
	if err := SetPermission("cron", PermissionSchedule, true); err != nil {
		t.Fatal(err)
	}
	if err := config.MarkStopped(); err != nil {
		t.Fatal(err)
	}
	RunDue(time.Now())()
	if _, err := os.Stat(filepath.Join(DataDirFor("cron"), "ran")); err == nil {
		t.Fatal("ran while lerd was stopped")
	}
	if err := config.ClearStopped(); err != nil {
		t.Fatal(err)
	}
	RunDue(time.Now())()
	if _, err := os.Stat(filepath.Join(DataDirFor("cron"), "ran")); err != nil {
		t.Fatal("did not run once lerd started again")
	}
}

// An answer covers the schedules it was given for: an update that adds one or
// changes how often or what runs asks again, and nothing runs meanwhile. An
// update that leaves them alone keeps the answer.
func TestScheduleState_AskedAgainWhenAnUpdateChangesTheSchedules(t *testing.T) {
	isolate(t)
	state := func() string {
		p, err := Find("cron")
		if err != nil {
			t.Fatal(err)
		}
		return PermissionState(p, PermissionSchedule)
	}
	update := func(schedules string) {
		writePlugin(t, Dir(), "cron", "api_version: 1\nname: cron\nversion: 0.2.0\nschedules:\n"+schedules)
	}
	schedulePlugin(t, "cron", "#!/bin/sh\n")
	if err := SetPermission("cron", PermissionSchedule, true); err != nil {
		t.Fatal(err)
	}

	update("  - {name: sweep, every: 10m, run: bin/run}\n")
	if got := state(); got != PermissionAllowed {
		t.Fatalf("an update with the same schedules: %q, want the answer kept", got)
	}
	for name, schedules := range map[string]string{
		"interval changed": "  - {name: sweep, every: 1m, run: bin/run}\n",
		"schedule added":   "  - {name: sweep, every: 10m, run: bin/run}\n  - {name: more, every: 1h, run: bin/run}\n",
		"script changed":   "  - {name: sweep, every: 10m, run: bin/other}\n",
	} {
		t.Run(name, func(t *testing.T) {
			schedulePlugin(t, "cron", "#!/bin/sh\n")
			if err := os.WriteFile(filepath.Join(Dir(), "cron", "bin", "other"), []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := SetPermission("cron", PermissionSchedule, true); err != nil {
				t.Fatal(err)
			}
			update(schedules)
			if got := state(); got != PermissionPending {
				t.Fatalf("state = %q, want pending again", got)
			}
		})
	}

	// A refusal is asked again too, since what was refused is not what it asks now.
	schedulePlugin(t, "cron", "#!/bin/sh\n")
	if err := SetPermission("cron", PermissionSchedule, false); err != nil {
		t.Fatal(err)
	}
	update("  - {name: sweep, every: 1h, run: bin/run}\n")
	if got := state(); got != PermissionPending {
		t.Fatalf("after a refusal and a change: %q, want pending", got)
	}
}
