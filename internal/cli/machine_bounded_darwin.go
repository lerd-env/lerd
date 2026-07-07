//go:build darwin

package cli

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/geodro/lerd/internal/podman"
)

// A podman machine subcommand blocks forever when the VM is wedged (post-sleep
// freeze). The self-heal runs on the hot path (FPM start, DNS watcher), so each
// subcommand is context-bounded to kill the child at its deadline. init and set
// stay unbounded: a first-run image pull is long and a config edit is instant.
const (
	machineQueryTimeout = 30 * time.Second // list, inspect
	machineStopTimeout  = 90 * time.Second
	machineStartTimeout = 3 * time.Minute
)

// machineCmdContext builds a context-bound podman command. A package var so
// darwin tests can substitute a fake process to exercise the timeout path.
var machineCmdContext = func(ctx context.Context, args ...string) *exec.Cmd {
	return podman.CmdContext(ctx, args...)
}

// machineQuery runs a read-only podman machine query (list, inspect), bounded by
// machineQueryTimeout, and returns its stdout.
func machineQuery(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), machineQueryTimeout)
	defer cancel()
	return machineCmdContext(ctx, args...).Output()
}

// runMachineStreaming runs a podman machine subcommand (stop, start) bounded by
// timeout, streaming its output. On timeout the context terminates the process
// so a wedged VM can't hang the caller.
func runMachineStreaming(timeout time.Duration, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := machineCmdContext(ctx, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
