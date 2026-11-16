package podman

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

func withRunningPs(t *testing.T, out string) *int {
	t.Helper()
	calls := 0
	prev := runningPsFn
	runningPsFn = func() (string, error) { calls++; return out, nil }
	t.Cleanup(func() { runningPsFn = prev; dropRunningMemo() })
	dropRunningMemo()
	return &calls
}

// A php run checks every service its site uses, and each podman inspect is a
// VM round trip on macOS, so the checks share one podman ps.
func TestRunningChecksShareOnePs(t *testing.T) {
	calls := withRunningPs(t, "lerd-mysql\t1792438625-3696\nlerd-redis\t1792438626-3744\n")

	if ok, _ := ContainerRunning("lerd-mysql"); !ok {
		t.Error("lerd-mysql is in the ps output, want running")
	}
	if !ContainerRunningQuiet("lerd-redis") {
		t.Error("lerd-redis is in the ps output, want running")
	}
	if got := ContainerRunID("lerd-redis"); got != "1792438626-3744" {
		t.Errorf("ContainerRunID = %q, want the ps start time and pid", got)
	}
	if *calls != 1 {
		t.Errorf("podman ps ran %d times, want 1", *calls)
	}
}

// A unit lerd just started or stopped must not be answered from the ps taken
// before it.
func TestRunningMemoDroppedOnUnitChange(t *testing.T) {
	calls := withRunningPs(t, "lerd-mysql\t1-2\n")

	ContainerRunning("lerd-mysql")
	notifyUnitChange("lerd-mysql")
	ContainerRunning("lerd-mysql")
	if *calls != 2 {
		t.Errorf("podman ps ran %d times, want a fresh one after the unit changed", *calls)
	}
}

// A container missing from a ps that succeeded is not running, so the daemon's
// sweeps over every worker of every site don't each cost an inspect.
func TestRunningMemoAnswersAbsentContainerWithoutInspect(t *testing.T) {
	withRunningPs(t, "lerd-mysql\t1-2\n")
	inspects := 0
	prev, prevCtx := execCommand, execCommandContext
	execCommand = func(name string, args ...string) *exec.Cmd { inspects++; return prev(name, args...) }
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		inspects++
		return prevCtx(ctx, name, args...)
	}
	t.Cleanup(func() { execCommand, execCommandContext = prev, prevCtx })

	if ok, _ := ContainerRunning("lerd-queue-acme"); ok {
		t.Error("lerd-queue-acme is not in the ps, want not running")
	}
	if containerRunningBounded("lerd-vite-acme") {
		t.Error("lerd-vite-acme is not in the ps, want not running")
	}
	if got := ContainerRunID("lerd-postgres"); got != "" {
		t.Errorf("ContainerRunID = %q for a container not in the ps, want empty", got)
	}
	if inspects != 0 {
		t.Errorf("ran %d podman inspects, want none", inspects)
	}
}

// When the ps itself failed the memo knows nothing, so each check still asks
// podman about its container directly.
func TestRunningMemoFallsBackWhenPsFails(t *testing.T) {
	prevPs := runningPsFn
	runningPsFn = func() (string, error) { return "", errors.New("vm stalled") }
	prev, prevCtx := execCommand, execCommandContext
	t.Cleanup(func() { runningPsFn = prevPs; execCommand, execCommandContext = prev, prevCtx; dropRunningMemo() })
	dropRunningMemo()

	execCommand = fakeExec("true", "", 0)
	if ok, _ := ContainerRunning("lerd-postgres"); !ok {
		t.Error("ContainerRunning: ps failed and inspect says running, want running")
	}
	execCommand = fakeExec("true 1-2", "", 0)
	if got := ContainerRunID("lerd-postgres"); got != "1-2" {
		t.Errorf("ContainerRunID = %q, want the run id inspect reports", got)
	}
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		fake := fakeExec("true", "", 0)(name, args...)
		cmd := exec.CommandContext(ctx, fake.Path, fake.Args[1:]...)
		cmd.Env = fake.Env
		return cmd
	}
	if !containerRunningBounded("lerd-postgres") {
		t.Error("containerRunningBounded: ps failed and inspect says running, want running")
	}
}
