//go:build linux

package cli

// migrateWorkersOnModeChange is a no-op on Linux: systemd always uses the
// exec path regardless of the configured mode, so there's nothing to
// reshape on disk. The `lerd workers mode` command still updates the
// config value for parity but no migration work is needed.
func migrateWorkersOnModeChange(_ /* fromMode */, _ /* toMode */ string) error {
	return nil
}

// migrateWorkersOnModeChangeStreaming is the streaming variant; on Linux
// we still emit the bracketing events so the dashboard's progress UI
// completes, then return immediately.
func migrateWorkersOnModeChangeStreaming(_ /* fromMode */, _ /* toMode */ string, emit func(WorkerModePhaseEvent)) error {
	if emit != nil {
		emit(WorkerModePhaseEvent{Phase: "done"})
	}
	return nil
}
