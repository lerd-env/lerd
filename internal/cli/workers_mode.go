package cli

import (
	"fmt"
	"runtime"
	"sync/atomic"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// workerMigrationActive is incremented while a worker-mode migration is in
// flight. Read by external supervisors (the exec-mode self-heal watcher in
// internal/watcher) so they don't race the migration's stop/start loop —
// e.g. a watcher tick that sees "no plist" mid-stop and tries to repair it
// would clobber the migration's pending start with a stale shape.
var workerMigrationActive atomic.Int32

// WorkerMigrationActive reports whether a worker-mode migration is currently
// running on this process. Nil-safe and zero-cost when not active.
func WorkerMigrationActive() bool { return workerMigrationActive.Load() > 0 }

// NewWorkersCmd returns the `lerd workers` parent command. Currently only
// `lerd workers mode [exec|container]` lives here, but the subcommand is
// structured as a group so future runtime-level options (concurrency,
// restart delay, ...) have an obvious home.
func NewWorkersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workers",
		Short: "Manage the worker runtime configuration (macOS-only for now)",
	}
	cmd.AddCommand(newWorkersModeCmd())
	return cmd
}

func newWorkersModeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mode [exec|container]",
		Short: "Show or set how framework workers are launched on macOS",
		Long: `Show or set the macOS worker runtime mode.

  exec       one podman exec per worker, supervised by launchd with a pid-file
             dedup guard. Lower memory; all workers share the FPM container's
             PHP process and OPcache. Default.

  container  one detached container per worker spawned from the FPM image.
             Higher memory; 1:1 supervisor boundary, more robust against
             podman-machine SSH bridge hiccups.

No argument prints the current value. The setting is ignored on Linux
which always uses exec-mode workers under systemd.

Changing the mode on macOS stops each active worker in its old shape,
cleans up the stale on-disk artifacts, and restarts it in the new shape.
No manual 'lerd stop && lerd start' needed.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			mode, show, err := workersModeFromArgs(args)
			if err != nil {
				return err
			}
			cfg, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			if show {
				return printWorkersMode(cfg)
			}
			prev := cfg.WorkerExecMode()
			if err := applyWorkersMode(mode, nil); err != nil {
				return err
			}
			feedback.Begin()
			if prev == mode {
				feedback.Line("worker mode already " + mode)
				return nil
			}
			feedback.Done("worker mode set to " + feedback.Val(mode) + " (was " + prev + ")")
			if runtime.GOOS == "darwin" {
				feedback.Note("active workers have been restarted in the new shape")
			} else {
				feedback.Note("Linux always uses the exec runtime; this setting only applies on macOS")
			}
			return nil
		},
	}
}

// workersModeFromArgs parses the user's `workers mode ...` argv. Returns
// (mode, show, err): `show` true means "no argument, print current".
func workersModeFromArgs(args []string) (mode string, show bool, err error) {
	if len(args) == 0 {
		return "", true, nil
	}
	switch args[0] {
	case config.WorkerExecModeExec, config.WorkerExecModeContainer:
		return args[0], false, nil
	}
	return "", false, fmt.Errorf("unknown mode %q, expected %q or %q",
		args[0], config.WorkerExecModeExec, config.WorkerExecModeContainer)
}

// BeforeWorkerMigration is invoked once at the start of a worker-mode
// migration with the unit names about to be touched. The UI registers a
// callback that cancels any open `podman logs -f` SSE streams for those
// units and pauses the container-state cache poller, so the migration's
// stop/rm/start podman calls don't compete with the daemon's own podman
// connections for gvproxy connection slots. nil on the CLI path (no UI
// streams to cancel, no cache poller running).
var BeforeWorkerMigration func(units []string)

// AfterWorkerMigration is invoked once when the migration loop ends,
// regardless of success. The UI uses it to resume the cache poller it
// paused in BeforeWorkerMigration.
var AfterWorkerMigration func()

// WorkerModePhaseEvent is one step in the migration. The dashboard streams
// these as NDJSON so the confirm modal can show real progress instead of
// a blind 30-60s spinner.
type WorkerModePhaseEvent struct {
	Phase   string `json:"phase"` // "saving_config" | "migrating_worker" | "done" | "error"
	Unit    string `json:"unit,omitempty"`
	Step    string `json:"step,omitempty"` // "stopping" | "cleaning" | "starting"
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ApplyWorkersMode is the exported wrapper around applyWorkersMode used
// by the dashboard handler so the web toggle drives the same migration
// path as the CLI. emit may be nil when called from the CLI; the streaming
// path is only used by the dashboard for live progress.
func ApplyWorkersMode(newMode string) error { return applyWorkersMode(newMode, nil) }

// ApplyWorkersModeStreaming is the streaming variant called by the web
// dashboard. Emits phase events at every meaningful step so the modal
// shows "Stopping lerd-horizon-parkapp", "Starting lerd-schedule-frontend",
// etc. rather than a blank spinner for the whole migration.
func ApplyWorkersModeStreaming(newMode string, emit func(WorkerModePhaseEvent)) error {
	return applyWorkersMode(newMode, emit)
}

// applyWorkersMode writes newMode to global config, then (on macOS, if
// the mode actually changed) stops every active worker in its old shape,
// removes stale on-disk artifacts, and restarts each in the new shape.
// On Linux it's a pure config write since workers always use exec under
// systemd. Idempotent for same-value writes.
func applyWorkersMode(newMode string, emit func(WorkerModePhaseEvent)) error {
	if emit == nil {
		emit = func(WorkerModePhaseEvent) {}
	}
	emit(WorkerModePhaseEvent{Phase: "saving_config"})
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	prev := cfg.WorkerExecMode()
	cfg.Workers.ExecMode = newMode
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}
	// migrateWorkersOnModeChange is a no-op on Linux (build-tag linked).
	// On macOS it executes the stop → clean → restart dance per worker.
	return migrateWorkersOnModeChangeStreaming(prev, newMode, emit)
}

func printWorkersMode(cfg *config.GlobalConfig) error {
	fmt.Printf("Worker mode: %s\n", cfg.WorkerExecMode())
	if runtime.GOOS != "darwin" {
		fmt.Println("  (Linux runs workers via podman exec under systemd; setting is informational.)")
	}
	return nil
}
