//go:build darwin

package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/geodro/lerd/internal/feedback"
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
	return streamMachine(timeout, nil, args...)
}

// startMachine runs `podman machine start` in its own process group. vfkit and
// gvproxy keep the group of whoever started them, so sharing the caller's group
// would take the VM down when that caller (an MCP session, a terminal) exits.
func startMachine() error {
	return streamMachine(machineStartTimeout, &syscall.SysProcAttr{Setpgid: true}, "machine", "start")
}

func streamMachine(timeout time.Duration, attr *syscall.SysProcAttr, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := machineCmdContext(ctx, args...)
	cmd.SysProcAttr = attr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// machineStatePoll is the gap between state checks; a var so tests skip it.
var machineStatePoll = 2 * time.Second

// stopMachineAndWait stops the VM and waits, at most wait in all, until podman
// reports it stopped. A stop can return, or time out, while the VM is still
// shutting down, and podman refuses `machine set` until it is fully down.
func stopMachineAndWait(name string, wait time.Duration) error {
	if err := runMachineStreaming(machineStopTimeout, "machine", "stop", name); err != nil {
		feedback.Warn("podman machine stop: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	state := "unknown"
	for ctx.Err() == nil {
		out, err := machineCmdContext(ctx, "machine", "inspect", "--format", "{{.State}}", name).Output()
		if err == nil {
			state = strings.TrimSpace(string(out))
		}
		if state == "stopped" {
			return nil
		}
		select {
		case <-ctx.Done():
		case <-time.After(machineStatePoll):
		}
	}
	return fmt.Errorf("the Podman Machine is still %s after being stopped, so its settings were not changed", state)
}
