package cli

import (
	"io/fs"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/term"
)

// A native run's children are not bound to it: pest-plugin-browser starts a
// Playwright server, and when the command exits that server survives as an
// orphan holding the inherited stdout, so a pipeline never sees EOF and each
// run leaks another one. Running in its own process group lets the group be
// reaped with the command.
func TestRunInOwnProcessGroupIsReaped(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "sleep 30 & exit 0")
	setOwnProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("getpgid: %v", err)
	}
	if pgid == syscall.Getpgrp() {
		t.Fatal("command must not share the caller's process group")
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}

	// The command is gone but its child is not, which is the leak.
	if syscall.Kill(-pgid, 0) != nil {
		t.Skip("the orphan exited on its own, nothing to assert")
	}

	reapProcessGroup(cmd)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(-pgid, 0) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the orphaned child survived the reap")
}

// An interactive run must stay in the caller's process group: a background group
// cannot read the terminal and would lose Ctrl-C, which tinker depends on.
func TestInteractiveRunKeepsTheCallersProcessGroup(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	grouped := groupNativeRun(cmd)

	if isInteractiveStdin() {
		if grouped || cmd.SysProcAttr != nil {
			t.Error("an interactive run must not be put in its own group")
		}
		return
	}
	if !grouped || cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Error("a non-interactive run must get its own group")
	}
}

// runHostAndReap must reap before propagating: the exit path calls os.Exit,
// which skips defers, so a deferred reap never ran and the Playwright server
// pest-plugin-browser started outlived every `lerd test`.
func TestRunHostAndReapKillsTheOrphanBeforeReturning(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "sleep 30 & exit 0")
	cmd.Stdin = nil

	pgidCh := make(chan int, 1)
	if err := runHostAndReap(cmd, func(c *exec.Cmd) { pgidCh <- c.Process.Pid }); err != nil {
		t.Fatalf("run: %v", err)
	}
	pid := <-pgidCh

	if !isInteractiveStdin() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if syscall.Kill(-pid, 0) != nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Error("the orphaned child survived the run")
	}
}

// /dev/null is a character device, so a file-mode test called every redirected
// run interactive and skipped the grouping such a run is exactly what needs.
func TestRedirectedStdinIsNotInteractive(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()

	info, err := devNull.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&fs.ModeCharDevice == 0 {
		t.Skip("this platform does not report /dev/null as a character device")
	}
	if term.IsTerminal(int(devNull.Fd())) {
		t.Error("/dev/null must not be treated as a terminal")
	}
}
