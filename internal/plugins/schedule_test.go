package plugins

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
	if got := readRuns("slow")["sweep"].Started; !got.Equal(start) {
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

// waitForFile waits for path to appear, failing the test after five seconds.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never appeared", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// slowPlugin installs an allowed plugin whose run writes "started" and then
// waits for a "go" file in its data folder.
func slowPlugin(t *testing.T, name string) {
	t.Helper()
	schedulePlugin(t, name, "#!/bin/sh\ntouch \"$LERD_PLUGIN_DATA/started\"\nwhile [ ! -e \"$LERD_PLUGIN_DATA/go\" ]; do sleep 0.05; done\n")
	if err := SetPermission(name, PermissionSchedule, true); err != nil {
		t.Fatal(err)
	}
}

// Whether a run is still going is read from lerd's record, so the CLI, a
// process apart from lerd-ui, says running rather than ok.
func TestScheduleStatuses_RunningIsSeenFromTheRecord(t *testing.T) {
	isolate(t)
	slowPlugin(t, "slow")
	wait := RunDue(time.Now())
	waitForFile(t, filepath.Join(DataDirFor("slow"), "started"))

	runningMu.Lock()
	saved := running
	running = map[string]*exec.Cmd{} // what another process sees
	runningMu.Unlock()
	p, _ := Find("slow")
	st := ScheduleStatuses(p)[0]
	runningMu.Lock()
	running = saved
	runningMu.Unlock()
	if !st.Running {
		t.Errorf("status mid-run = %+v, want running", st)
	}

	_ = os.WriteFile(filepath.Join(DataDirFor("slow"), "go"), nil, 0o644)
	wait()
	if st := ScheduleStatuses(p)[0]; st.Running || st.LastError != "" {
		t.Errorf("status after = %+v, want finished ok", st)
	}
}

// A run that ends after its plugin was removed leaves no record behind, so a
// reinstall starts afresh.
func TestRunDue_ARunEndingAfterRemovalLeavesNoRecord(t *testing.T) {
	isolate(t)
	slowPlugin(t, "slow")
	wait := RunDue(time.Now())
	waitForFile(t, filepath.Join(DataDirFor("slow"), "started"))
	if err := Remove("slow"); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(DataDirFor("slow"), 0o755)
	_ = os.WriteFile(filepath.Join(DataDirFor("slow"), "go"), nil, 0o644)
	wait()
	if runs := readRuns("slow"); len(runs) != 0 {
		t.Errorf("record after removal = %+v, want none", runs)
	}
}

// Shutting lerd-ui down stops the runs it started, which on macOS would
// otherwise outlive it in their own process group.
func TestStopSchedules_KillsRunsInProgress(t *testing.T) {
	isolate(t)
	resumeAfterStop(t)
	slowPlugin(t, "slow")
	wait := RunDue(time.Now())
	waitForFile(t, filepath.Join(DataDirFor("slow"), "started"))
	done := make(chan struct{})
	StopSchedules()
	// Recorded before StopSchedules returns, since lerd-ui exits right after.
	p, _ := Find("slow")
	if st := ScheduleStatuses(p)[0]; st.Running || st.LastError == "" {
		t.Errorf("status after shutdown = %+v, want stopped with a reason", st)
	}
	go func() { wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = os.WriteFile(filepath.Join(DataDirFor("slow"), "go"), nil, 0o644)
		t.Fatal("the run kept going after StopSchedules")
	}
}

// Every surface showing schedules hears when a run starts and ends.
func TestRunDue_TellsWhenARunStartsAndEnds(t *testing.T) {
	isolate(t)
	schedulePlugin(t, "cron", "#!/bin/sh\n")
	if err := SetPermission("cron", PermissionSchedule, true); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	calls := 0
	SetScheduleNotifier(func() { mu.Lock(); calls++; mu.Unlock() })
	t.Cleanup(func() { SetScheduleNotifier(func() {}) })
	RunDue(time.Now())()
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("notified %d times, want at start and end", calls)
	}
}

// An answer applies to the schedules the user was shown: one that changed
// while the question was open is refused rather than approved unseen.
func TestAnswerAsShown_RefusesSchedulesTheUserDidNotSee(t *testing.T) {
	isolate(t)
	schedulePlugin(t, "cron", "#!/bin/sh\n")
	p, _ := Find("cron")
	shown := Scope(p, PermissionSchedule)
	writePlugin(t, Dir(), "cron", "api_version: 1\nname: cron\nschedules:\n  - {name: sweep, every: 1m, run: bin/run}\n")
	if err := AnswerAsShown("cron", PermissionSchedule, true, shown); err == nil {
		t.Fatal("approved schedules the user was never shown")
	}
	if p, _ := Find("cron"); PermissionState(p, PermissionSchedule) != PermissionPending {
		t.Fatal("a refused answer must leave it pending")
	}
	p, _ = Find("cron")
	if err := AnswerAsShown("cron", PermissionSchedule, true, Scope(p, PermissionSchedule)); err != nil {
		t.Fatal(err)
	}
	if PermissionState(p, PermissionSchedule) != PermissionAllowed {
		t.Fatal("the answer to what was shown should hold")
	}
}

// An update landing between the check and the save leaves the answer covering
// only what was shown, so the plugin is asked again rather than approved.
func TestAnswerAsShown_UpdateDuringTheAnswerIsAskedAgain(t *testing.T) {
	isolate(t)
	schedulePlugin(t, "cron", "#!/bin/sh\n")
	p, _ := Find("cron")
	shown := Scope(p, PermissionSchedule)
	answerChecked = func() {
		writePlugin(t, Dir(), "cron", "api_version: 1\nname: cron\nschedules:\n  - {name: sweep, every: 1m, run: bin/run}\n")
	}
	t.Cleanup(func() { answerChecked = func() {} })
	if err := AnswerAsShown("cron", PermissionSchedule, true, shown); err != nil {
		t.Fatal(err)
	}
	p, _ = Find("cron")
	if got := PermissionState(p, PermissionSchedule); got != PermissionPending {
		t.Fatalf("state = %q, want pending: the answer covered the schedules shown, not the update", got)
	}
}

// resumeAfterStop lets the next test run schedules again after one that shut
// them down.
func resumeAfterStop(t *testing.T) {
	t.Cleanup(func() {
		runningMu.Lock()
		stopping = false
		runningMu.Unlock()
	})
}

// A shutdown landing between a run's start and its registration still stops
// it, and once shutdown began no new run starts.
func TestStopSchedules_CatchesARunStartingDuringShutdown(t *testing.T) {
	isolate(t)
	resumeAfterStop(t)
	slowPlugin(t, "slow")
	afterStart = func() {
		go StopSchedules()
		time.Sleep(200 * time.Millisecond)
	}
	t.Cleanup(func() { afterStart = func() {} })
	done := make(chan struct{})
	go func() { RunDue(time.Now())(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = os.WriteFile(filepath.Join(DataDirFor("slow"), "go"), nil, 0o644)
		t.Fatal("a run starting during shutdown kept going")
	}

	afterStart = func() {}
	_ = os.Remove(filepath.Join(DataDirFor("slow"), "started"))
	RunDue(time.Now().Add(time.Hour))()
	if _, err := os.Stat(filepath.Join(DataDirFor("slow"), "started")); err == nil {
		t.Fatal("a run started after shutdown began")
	}
}
