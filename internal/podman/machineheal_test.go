package podman

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// fakeExecContextExit returns an execCommandContext drop-in whose child exits
// with exitFn()'s code each call, so probe responsiveness can be toggled over
// successive calls without a real podman.
func fakeExecContextExit(exitFn func() int) func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := append([]string{"-test.run=TestHelperProcess", "--", name}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"HELPER_EXIT="+strconv.Itoa(exitFn()),
		)
		return cmd
	}
}

// resetHealState restores the package heal globals so tests don't leak into
// each other through lastHealAt / MachineHeal / execCommandContext.
func resetHealState(t *testing.T) {
	t.Helper()
	prevCtx := execCommandContext
	prevHeal := MachineHeal
	t.Cleanup(func() {
		execCommandContext = prevCtx
		MachineHeal = prevHeal
		healMu.Lock()
		lastHealAt = time.Time{}
		healMu.Unlock()
	})
	healMu.Lock()
	lastHealAt = time.Time{}
	healMu.Unlock()
}

func TestEnsureMachineResponsiveHealthy(t *testing.T) {
	resetHealState(t)
	execCommandContext = fakeExecContextExit(func() int { return 0 })
	healed := 0
	MachineHeal = func() { healed++ }

	if err := EnsureMachineResponsive(); err != nil {
		t.Fatalf("healthy machine: got error %v", err)
	}
	if healed != 0 {
		t.Errorf("healthy machine healed %d times, want 0", healed)
	}
}

func TestEnsureMachineResponsiveHealsThenRecovers(t *testing.T) {
	resetHealState(t)
	// First probe fails; the heal "fixes" it so the second probe succeeds.
	stalled := true
	execCommandContext = fakeExecContextExit(func() int {
		if stalled {
			return 1
		}
		return 0
	})
	healed := 0
	MachineHeal = func() { healed++; stalled = false }

	if err := EnsureMachineResponsive(); err != nil {
		t.Fatalf("post-heal recovery: got error %v", err)
	}
	if healed != 1 {
		t.Errorf("healed %d times, want 1", healed)
	}
}

func TestEnsureMachineResponsiveDeadMachineErrors(t *testing.T) {
	resetHealState(t)
	execCommandContext = fakeExecContextExit(func() int { return 1 })
	healed := 0
	MachineHeal = func() { healed++ } // heal doesn't fix it

	if err := EnsureMachineResponsive(); err == nil {
		t.Fatal("dead machine: want error, got nil")
	}
	if healed != 1 {
		t.Errorf("healed %d times, want 1 (retry-once)", healed)
	}
}

func TestEnsureMachineResponsiveCooldownBlocksSecondHeal(t *testing.T) {
	resetHealState(t)
	execCommandContext = fakeExecContextExit(func() int { return 1 })
	healed := 0
	MachineHeal = func() { healed++ }

	_ = EnsureMachineResponsive() // heals once, still dead
	if err := EnsureMachineResponsive(); err == nil {
		t.Fatal("second call: want error, got nil")
	}
	if healed != 1 {
		t.Errorf("healed %d times across two calls, want 1 (cooldown)", healed)
	}
}

func TestEnsureMachineResponsiveNoHealHookIsNoop(t *testing.T) {
	resetHealState(t)
	probed := 0
	execCommandContext = fakeExecContextExit(func() int { probed++; return 1 })
	MachineHeal = nil // Linux: no machine VM

	if err := EnsureMachineResponsive(); err != nil {
		t.Fatalf("no heal hook (Linux): want nil no-op, got %v", err)
	}
	if probed != 0 {
		t.Errorf("no heal hook must skip the probe, probed %d times", probed)
	}
}
