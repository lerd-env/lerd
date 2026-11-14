package plugins

import (
	"encoding/json"
	"fmt"
	"os"
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

type scheduleRun struct {
	LastRun   time.Time `json:"last_run"`
	LastError string    `json:"last_error,omitempty"`
}

var (
	// running holds the plugin/schedule pairs mid-run, so a slow run is never
	// started a second time by the next tick.
	runningMu sync.Mutex
	running   = map[string]bool{}

	// runsMu serialises the read-modify-write of the runs file.
	runsMu sync.Mutex
)

// runsPath is lerd's record of when each schedule last ran. It lives in lerd's
// data dir rather than the plugin's, which the plugin can write.
func runsPath() string { return filepath.Join(config.DataDir(), "plugin-schedules.json") }

// RunDue starts every allowed schedule whose interval has passed since it last
// started, each on its own, and returns a func that waits for the ones it
// started. Nothing runs while lerd is stopped.
func RunDue(now time.Time) (wait func()) {
	// lerd stop pauses them, the way it stops lerd's own scheduled workers.
	if config.IsStopped() {
		return func() {}
	}
	list, _ := Load()
	runs := readRuns()
	var wg sync.WaitGroup
	for _, p := range list {
		if len(p.Schedules) == 0 || PermissionState(p, PermissionSchedule) != PermissionAllowed {
			continue
		}
		for _, s := range p.Schedules {
			if !due(runs[p.Name][s.Name].LastRun, s.Interval(), now) || !claim(p.Name, s.Name) {
				continue
			}
			// Recorded before it runs, so lerd-ui restarting mid-run does not
			// start it again at the next tick.
			recordRun(p.Name, s.Name, scheduleRun{LastRun: now})
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer release(p.Name, s.Name)
				p.runSchedule(s, now)
			}()
		}
	}
	return wg.Wait
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
	if running[key] {
		return false
	}
	running[key] = true
	return true
}

func release(plugin, schedule string) {
	runningMu.Lock()
	defer runningMu.Unlock()
	delete(running, plugin+"/"+schedule)
}

func isRunning(plugin, schedule string) bool {
	runningMu.Lock()
	defer runningMu.Unlock()
	return running[plugin+"/"+schedule]
}

func (p *Plugin) runSchedule(s Schedule, started time.Time) {
	var out Tail
	cmd, err := p.Command(s.Run, SurfaceSchedule, nil, map[string]string{"LERD_SCHEDULE": s.Name})
	if err == nil {
		cmd.Dir = p.Dir
		cmd.Stdout, cmd.Stderr = &out, &out
		err = RunWithin(cmd, s.Limit())
	}

	status, lastError := "ok", ""
	if err != nil {
		status = err.Error()
		lastError = strings.TrimSpace(out.String() + "\n" + status)
		Failed(Failure{Plugin: p.Name, Surface: SurfaceSchedule, What: s.Name, Error: lastError})
	}
	recordRun(p.Name, s.Name, scheduleRun{LastRun: started, LastError: lastError})

	f, ferr := os.OpenFile(filepath.Join(DataDirFor(p.Name), ScheduleLog), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if ferr != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s %s: %s\n%s", started.Format(time.RFC3339), s.Name, s.Run, status, out.String())
}

// ScheduleStatuses reports each of the plugin's schedules with when it last
// ran, how that went and when it runs next.
func ScheduleStatuses(p *Plugin) []ScheduleStatus {
	runs := readRuns()[p.Name]
	out := make([]ScheduleStatus, 0, len(p.Schedules))
	for _, s := range p.Schedules {
		r := runs[s.Name]
		st := ScheduleStatus{Name: s.Name, Every: s.Every, LastRun: r.LastRun, LastError: r.LastError, Running: isRunning(p.Name, s.Name)}
		if !r.LastRun.IsZero() {
			st.NextRun = r.LastRun.Add(s.Interval())
		}
		out = append(out, st)
	}
	return out
}

func readRuns() map[string]map[string]scheduleRun {
	runsMu.Lock()
	defer runsMu.Unlock()
	return readRunsLocked()
}

func readRunsLocked() map[string]map[string]scheduleRun {
	runs := map[string]map[string]scheduleRun{}
	if data, err := os.ReadFile(runsPath()); err == nil {
		_ = json.Unmarshal(data, &runs)
	}
	return runs
}

func recordRun(plugin, schedule string, r scheduleRun) {
	runsMu.Lock()
	defer runsMu.Unlock()
	runs := readRunsLocked()
	if runs[plugin] == nil {
		runs[plugin] = map[string]scheduleRun{}
	}
	runs[plugin][schedule] = r
	writeRunsLocked(runs)
}

// forgetScheduleRuns drops the plugin's record, so a reinstall starts afresh.
func forgetScheduleRuns(plugin string) {
	runsMu.Lock()
	defer runsMu.Unlock()
	runs := readRunsLocked()
	if _, ok := runs[plugin]; !ok {
		return
	}
	delete(runs, plugin)
	writeRunsLocked(runs)
}

func writeRunsLocked(runs map[string]map[string]scheduleRun) {
	data, err := json.Marshal(runs)
	if err != nil {
		return
	}
	path := runsPath()
	config.GuardRealWrite(path)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, data, 0o644)
}
