//go:build darwin

package cli

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestMachineHelperProcess is the child spawned by the fake machineCmdContext.
// It sleeps far longer than any test timeout so the parent's context deadline is
// what ends it, proving the bound terminates a wedged command.
func TestMachineHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MACHINE_HELPER") != "1" {
		return
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

func fakeHungMachineCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestMachineHelperProcess", "--")
	cmd.Env = append(os.Environ(), "GO_WANT_MACHINE_HELPER=1")
	return cmd
}

// runMachineStreaming must return promptly when the underlying command hangs,
// instead of blocking the hot path (FPM start, DNS watcher) on a wedged VM.
func TestRunMachineStreamingBoundsAHungCommand(t *testing.T) {
	prev := machineCmdContext
	t.Cleanup(func() { machineCmdContext = prev })
	machineCmdContext = fakeHungMachineCmd

	start := time.Now()
	err := runMachineStreaming(200*time.Millisecond, "machine", "stop", "x")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error when the machine command is killed by the deadline")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("runMachineStreaming took %v; expected it to return near the 200ms bound", elapsed)
	}
}
