//go:build windows

package cli

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func startWinSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	ps, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("powershell not found")
	}
	cmd := exec.Command(ps, "-NoProfile", "-Command", "Start-Sleep 60")
	setOwnProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd
}

func TestKillPIDEndsTheProcessTree(t *testing.T) {
	cmd := startWinSleeper(t)
	pid := cmd.Process.Pid
	if !processExists(pid) || killPID(pid, 0) != nil {
		t.Fatal("a running process must probe as alive")
	}
	if !leadsOwnProcessGroup(cmd) {
		t.Error("setOwnProcessGroup not visible through leadsOwnProcessGroup")
	}
	if err := killPID(pid, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	for i := 0; i < 50 && processExists(pid); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if processExists(pid) {
		t.Error("process still alive after killPID")
	}
	if killPID(pid, 0) == nil {
		t.Error("probing a dead pid must fail")
	}
}

func TestProcessExistsRejectsBadPIDs(t *testing.T) {
	if processExists(0) || processExists(-4) {
		t.Error("non-positive pids are never live")
	}
}
