package lifecycle

import (
	"io"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
)

// Swappable for tests so the teardown order can be asserted without stopping
// real units or shelling out to podman.
var (
	stopUnitFn          = podman.StopUnit
	stopPodmanMachineFn = StopPodmanMachine
	batchStopFn         = BatchStopContainers
)

// Stop tears down every container, service, and worker unit, running the
// per-unit stops through runner. skip removes units the caller must not stop.
func Stop(runner ParallelRunner, skip ...string) error {
	units := StopUnitSet(skip...)

	feedback.Begin()
	feedback.Line("stopping lerd")

	// Mark the intentional shutdown before tearing anything down, so the worker
	// health watcher (which keeps running) suppresses heal/notification noise for
	// the workers we're about to stop. They stay enabled and come back on start.
	_ = config.MarkStopped()

	// On macOS: stop all containers in one podman call before the parallel
	// per-unit jobs run. This avoids serialising N individual podman stop
	// requests through the Podman Machine socket (which can take 5s × N).
	batchStopFn(units)

	_ = runner(stopJobs(units))
	return nil
}

// stopJobs turns units into one stop job each, labelled without the lerd-
// prefix the way the spinner lists them.
func stopJobs(units []string) []Job {
	jobs := make([]Job, len(units))
	for i, u := range units {
		unit := u
		label := strings.TrimSuffix(strings.TrimPrefix(unit, "lerd-"), ".timer")
		jobs[i] = Job{
			Label: label,
			Run:   func(w io.Writer) error { return stopUnitFn(unit) },
		}
	}
	return jobs
}

// Quit is the full teardown behind `lerd quit`: everything Stop covers, then
// the host process units, then the Podman Machine VM. skip removes units the
// caller must not stop. beforeMachineStop, when set, runs after the process
// units and before the VM, for host cleanup that must not wait out a VM stop
// that takes seconds; pass nil when there is none.
func Quit(runner ParallelRunner, beforeMachineStop func(), skip ...string) error {
	if err := Stop(runner, skip...); err != nil {
		return err
	}
	// Stop leaves worktree workers to the watcher, which brings them back on
	// the next start; quit takes the watcher down too, so it stops them here.
	// The ssh-agent is left up by Stop so its unlocked keys survive a restart;
	// quit is the full teardown, so it goes too.
	quitOnly := RegisteredWorktreeWorkerUnits()
	if podman.QuadletInstalled(podman.SSHAgentUnit) {
		quitOnly = append(quitOnly, podman.SSHAgentUnit)
	}
	_ = runner(stopJobs(quitOnly))
	stopProcessUnits(QuitProcessUnits(skip...))
	if beforeMachineStop != nil {
		beforeMachineStop()
	}
	stopPodmanMachineFn()
	return nil
}

// ShutdownForLogout is the teardown the watcher runs when the OS is logging out
// or restarting. It differs from Quit in two ways that both matter only when
// the shutdown is driven from inside lerd-watcher.
//
// It never stops podman.WatcherUnit, and it stops the Podman Machine VM before the
// host process units rather than after. The VM is the only step whose loss
// costs anything: a database killed mid-write replays its write-ahead log for
// minutes on the next start. lerd-ui, lerd-dns and lerd-tray are host processes
// that launchd is terminating anyway, so they are the right thing to leave
// until last if the exit grace runs out.
func ShutdownForLogout(runner ParallelRunner) error {
	if err := Stop(runner, podman.WatcherUnit); err != nil {
		return err
	}
	stopPodmanMachineFn()
	stopProcessUnits(QuitProcessUnits(podman.WatcherUnit))
	return nil
}

// stopProcessUnits stops host process units in order, reporting each one.
func stopProcessUnits(units []string) {
	for _, unit := range units {
		s := feedback.Start("stopping " + unit)
		if err := stopUnitFn(unit); err != nil {
			s.Fail(err)
		} else {
			s.OK("")
		}
	}
}

// StopWorktreeWorkerUnits stops a worktree's worker units without removing
// them. Removing a worktree calls it before git deletes the tree: a worker
// still running writes into it (Vite's cache) and makes the removal fail.
func StopWorktreeWorkerUnits(siteName, wtBase string) {
	for _, u := range WorktreeWorkerUnits(siteName, wtBase) {
		_ = stopUnitFn(u)
	}
}
