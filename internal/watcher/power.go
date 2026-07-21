package watcher

import (
	"time"

	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/power"
)

// powerRestartCooldown is the shortest gap between two power-driven restart
// rounds. Unplugging and replugging a laptop in quick succession should not
// bounce every reload worker each time, and the cadence being corrected is a
// background nicety, not something worth a restart storm over.
const powerRestartCooldown = 5 * time.Minute

// powerWatchState tracks the last observed power state and when workers were
// last bounced for it. Split out from the loop so the transition and cooldown
// rules can be tested without a battery to unplug.
type powerWatchState struct {
	last        power.State
	lastRestart time.Time
	cooldown    time.Duration
}

// shouldRestart reports whether a transition to cur warrants restarting the
// polling reload workers. A change suppressed by the cooldown deliberately
// leaves `last` untouched so the next tick retries it: the workers are still
// running the previous state's interval, and dropping the transition would
// strand them there until the power source changed again.
func (s *powerWatchState) shouldRestart(cur power.State, now time.Time) bool {
	if cur == s.last {
		return false
	}
	if !s.lastRestart.IsZero() && now.Sub(s.lastRestart) < s.cooldown {
		return false
	}
	s.last = cur
	s.lastRestart = now
	return true
}

// Seams for tests, so the loop can be driven without real hardware or workers.
var (
	powerCurrentFn = power.Current
	powerRestartFn = restartPollingReloadWorkers
	hostCanPollFn  = config.HostCanPollWatchers
)

// WatchPower re-applies the reload watcher's poll interval when the machine's
// power source changes. The interval is baked into a worker's unit when it is
// written and chokidar reads it once at startup, so without this an unplugged
// laptop would keep polling at the mains cadence until something else happened
// to restart the worker.
//
// Hosts where no watcher can ever poll return immediately: there would be no
// interval to re-apply, and ticking anyway costs a probe every interval and
// logs a power transition that changed nothing.
func WatchPower(interval time.Duration) {
	if !hostCanPollFn() {
		return
	}
	state := &powerWatchState{last: powerCurrentFn(), cooldown: powerRestartCooldown}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		cur := powerCurrentFn()
		if !state.shouldRestart(cur, time.Now()) {
			continue
		}
		logger.Info("power state changed, re-applying reload watcher cadence", "state", cur.String())
		powerRestartFn(cur)
	}
}

// restartPollingReloadWorkers rewrites and restarts every worker whose reload
// watcher is polling, so it picks up the interval for the new power state.
//
// Only workers that are currently active are touched: this must never
// resurrect a worker the user deliberately stopped, and a worker that is down
// will pick up the new interval whenever it is next started anyway.
func restartPollingReloadWorkers(state power.State) {
	if config.IsStopped() || cli.WorkerMigrationActive() {
		return
	}
	reg, err := config.LoadSites()
	if err != nil || reg == nil {
		return
	}
	cfg, _ := config.LoadGlobal()
	defaultPHP := ""
	if cfg != nil {
		defaultPHP = cfg.PHP.DefaultVersion
	}

	for _, s := range reg.Sites {
		if s.Ignored || s.Paused {
			continue
		}
		// Where the watcher can see host events it never polls, so there is
		// no interval to re-apply and nothing to restart.
		if !config.WatcherNeedsPolling(s.Path) || !config.ProjectHasChokidar(s.Path) {
			continue
		}
		proj, _ := config.LoadProjectConfig(s.Path)
		if proj == nil || len(proj.ReloadWorkers) == 0 {
			continue
		}
		fw, ok := config.GetFrameworkForDir(s.Framework, s.Path)
		if !ok || fw == nil {
			continue
		}
		paused := make(map[string]bool, len(s.PausedWorkers))
		for _, name := range s.PausedWorkers {
			paused[name] = true
		}
		php := s.PHPVersion
		if php == "" {
			php = defaultPHP
		}

		for _, name := range proj.ReloadWorkers {
			if paused[name] {
				continue
			}
			def, ok := fw.Workers[name]
			if !ok || def.ReloadCommand == "" {
				continue
			}
			if supported, _ := cli.WorkerSupportedOnPlatform(def); !supported {
				continue
			}
			unit := cli.WorkerUnitName(s.Name, s.Path, name)
			if status, _ := podman.UnitStatus(unit); status != "active" {
				continue
			}
			logger.Info("restarting reload worker for new power state",
				"unit", unit, "state", state.String())
			if err := cli.WorkerStartForSite(s.Name, s.Path, php, name, def, true); err != nil {
				logger.Error("power-driven worker restart failed", "unit", unit, "err", err)
			}
		}
	}
}
