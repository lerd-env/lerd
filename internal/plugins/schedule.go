package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// ScheduleLog is the file in a plugin's data dir that collects its scheduled
// runs' output.
const ScheduleLog = "schedule.log"

// ScheduleStatus is one schedule as every surface shows it. NextRun is zero
// when it has never run, which means at the next tick once allowed.
type ScheduleStatus struct {
	Name      string    `json:"name"`
	Every     string    `json:"every"`
	LastRun   time.Time `json:"last_run,omitzero"`
	LastError string    `json:"last_error,omitempty"`
	NextRun   time.Time `json:"next_run,omitzero"`
	Running   bool      `json:"running,omitempty"`
}

// scheduleRun is lerd's record of a schedule's last run. A Finished before
// Started means it is still going, which any process can read.
type scheduleRun struct {
	Started   time.Time `json:"started"`
	Finished  time.Time `json:"finished,omitzero"`
	LastError string    `json:"last_error,omitempty"`
}

var (
	// running holds the runs lerd-ui started and has not seen end, so a slow
	// one is never started twice and StopSchedules can kill them.
	runningMu sync.Mutex
	running   = map[string]*exec.Cmd{}

	// active counts the runs not yet recorded as ended, for StopSchedules.
	active sync.WaitGroup

	// runsMu serialises the read-modify-write of a plugin's record file.
	runsMu sync.Mutex

	scheduleChanged = func() {}
)

// SetScheduleNotifier sets what is told when a run starts or ends, so the
// dashboard can refresh what it shows.
func SetScheduleNotifier(fn func()) { scheduleChanged = fn }

// runsPath is lerd's record of a plugin's schedule runs. It lives in lerd's
// data dir rather than the plugin's, which the plugin can write, one file per
// plugin so a removal never races a write for another.
func runsPath(plugin string) string {
	return filepath.Join(config.DataDir(), "plugin-schedules", plugin+".json")
}

// RunDue starts every allowed schedule whose interval has passed since it last
// started, each on its own, and returns a func that waits for the ones it
// started. Nothing runs while lerd is stopped.
func RunDue(now time.Time) (wait func()) {
	// lerd stop pauses them, the way it stops lerd's own scheduled workers.
	if config.IsStopped() {
		return func() {}
	}
	list, _ := Load()
	var wg sync.WaitGroup
	for _, p := range list {
		if len(p.Schedules) == 0 || PermissionState(p, PermissionSchedule) != PermissionAllowed {
			continue
		}
		runs := readRuns(p.Name)
		for _, s := range p.Schedules {
			if !due(runs[s.Name].Started, s.Interval(), now) || !claim(p.Name, s.Name) {
				continue
			}
			// Recorded before it runs, so lerd-ui restarting mid-run does not
			// start it again; a run whose start cannot be kept is not started.
			if err := recordStart(p.Name, s.Name, now); err != nil {
				release(p.Name, s.Name)
				Failed(Failure{Plugin: p.Name, Surface: SurfaceSchedule, What: s.Name, Error: "could not record the run: " + err.Error()})
				continue
			}
			scheduleChanged()
			wg.Add(1)
			active.Add(1)
			go func() {
				defer wg.Done()
				defer active.Done()
				defer scheduleChanged()
				defer release(p.Name, s.Name)
				p.runSchedule(s, now)
			}()
		}
	}
	return wg.Wait
}

// StopSchedules kills every run lerd-ui started, for its shutdown: each runs in
// its own process group, which outlives lerd-ui on macOS. It waits a moment for
// the runs to record that they were stopped, since lerd-ui exits right after.
func StopSchedules() {
	runningMu.Lock()
	for _, cmd := range running {
		if cmd != nil {
			KillProcessGroup(cmd)
		}
	}
	runningMu.Unlock()
	done := make(chan struct{})
	go func() { active.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

// due is true for a schedule that never ran, whose interval has passed, or
// whose last run sits in the future because the clock was set back.
func due(last time.Time, every time.Duration, now time.Time) bool {
	return last.IsZero() || last.After(now) || now.Sub(last) >= every
}

func claim(plugin, schedule string) bool {
	runningMu.Lock()
	defer runningMu.Unlock()
	key := plugin + "/" + schedule
	if _, ok := running[key]; ok {
		return false
	}
	running[key] = nil
	return true
}

func started(plugin, schedule string, cmd *exec.Cmd) {
	runningMu.Lock()
	defer runningMu.Unlock()
	running[plugin+"/"+schedule] = cmd
}

func release(plugin, schedule string) {
	runningMu.Lock()
	defer runningMu.Unlock()
	delete(running, plugin+"/"+schedule)
}

func (p *Plugin) runSchedule(s Schedule, at time.Time) {
	var out Tail
	cmd, err := p.Command(s.Run, SurfaceSchedule, nil, map[string]string{"LERD_SCHEDULE": s.Name})
	if err == nil {
		cmd.Dir = p.Dir
		cmd.Stdout, cmd.Stderr = &out, &out
		var wait func() error
		if wait, err = startWithin(cmd, s.Limit()); err == nil {
			started(p.Name, s.Name, cmd)
			err = wait()
		}
	}

	status, lastError := "ok", ""
	if err != nil {
		status = err.Error()
		lastError = strings.TrimSpace(out.String() + "\n" + status)
		Failed(Failure{Plugin: p.Name, Surface: SurfaceSchedule, What: s.Name, Error: lastError})
	}
	recordFinish(p.Name, s.Name, at, lastError)

	f, ferr := os.OpenFile(filepath.Join(DataDirFor(p.Name), ScheduleLog), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if ferr != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s %s: %s\n%s", at.Format(time.RFC3339), s.Name, s.Run, status, out.String())
}

// ScheduleStatuses reports each of the plugin's schedules with when it last
// ran, how that went and when it runs next. A run still unfinished past its
// limit was cut off with lerd-ui, and says so.
func ScheduleStatuses(p *Plugin) []ScheduleStatus {
	runs := readRuns(p.Name)
	now := time.Now()
	out := make([]ScheduleStatus, 0, len(p.Schedules))
	for _, s := range p.Schedules {
		r := runs[s.Name]
		st := ScheduleStatus{Name: s.Name, Every: s.Every, LastRun: r.Started, LastError: r.LastError}
		if !r.Started.IsZero() {
			st.NextRun = r.Started.Add(s.Interval())
			if r.Finished.Before(r.Started) {
				if now.Sub(r.Started) < s.Limit() {
					st.Running = true
				} else {
					st.LastError = "stopped before it finished"
				}
			}
		}
		out = append(out, st)
	}
	return out
}

func readRuns(plugin string) map[string]scheduleRun {
	runsMu.Lock()
	defer runsMu.Unlock()
	return readRunsLocked(plugin)
}

func readRunsLocked(plugin string) map[string]scheduleRun {
	runs := map[string]scheduleRun{}
	if data, err := os.ReadFile(runsPath(plugin)); err == nil {
		_ = json.Unmarshal(data, &runs)
	}
	return runs
}

func recordStart(plugin, schedule string, at time.Time) error {
	runsMu.Lock()
	defer runsMu.Unlock()
	runs := readRunsLocked(plugin)
	runs[schedule] = scheduleRun{Started: at}
	return writeRunsLocked(plugin, runs)
}

// recordFinish completes the record its run started. A record that is gone, or
// that a newer start replaced, means the plugin was removed or reinstalled
// meanwhile, and is left alone.
func recordFinish(plugin, schedule string, at time.Time, lastError string) {
	runsMu.Lock()
	defer runsMu.Unlock()
	runs := readRunsLocked(plugin)
	r, ok := runs[schedule]
	if !ok || !r.Started.Equal(at) {
		return
	}
	r.Finished, r.LastError = time.Now(), lastError
	runs[schedule] = r
	_ = writeRunsLocked(plugin, runs)
}

// forgetScheduleRuns drops the plugin's record, so a reinstall starts afresh.
func forgetScheduleRuns(plugin string) {
	runsMu.Lock()
	defer runsMu.Unlock()
	path := runsPath(plugin)
	config.GuardRealWrite(path)
	_ = os.Remove(path)
}

// writeRunsLocked replaces the file whole, so a crash mid-write leaves the
// previous record rather than a truncated one.
func writeRunsLocked(plugin string, runs map[string]scheduleRun) error {
	data, err := json.Marshal(runs)
	if err != nil {
		return err
	}
	path := runsPath(plugin)
	config.GuardRealWrite(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), plugin+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
