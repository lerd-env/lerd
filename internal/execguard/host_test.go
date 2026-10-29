package execguard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// orphanMaker is a host command that leaves a child running after it exits, the
// way pest-plugin-browser leaves its Playwright server, and writes its pid out.
func orphanMaker(t *testing.T) (*exec.Cmd, string) {
	t.Helper()
	pidFile := filepath.Join(t.TempDir(), "pid")
	return exec.Command("sh", "-c", "sleep 30 & echo $! > "+pidFile+"; wait"), pidFile
}

func readPid(t *testing.T, path string) int {
	t.Helper()
	for i := 0; i < 50; i++ {
		if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) > 0 {
			pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the command never wrote its child's pid")
	return 0
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// A native run's children are not bound to it: the Playwright server outlives
// a passing `lerd pest` and keeps listening, one more for every run.
func TestRun_hostRunTakesWhatItLeftBehindWithIt(t *testing.T) {
	dir := stubDir(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	cmd := exec.Command("sh", "-c", "sleep 30 & echo $! > "+pidFile+"; exit 0")

	if err := Run(context.Background(), cmd, ""); err != nil {
		t.Fatal(err)
	}
	if child := readPid(t, pidFile); alive(child) {
		_ = syscall.Kill(child, syscall.SIGKILL)
		t.Fatal("the child the run left behind is still running")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a finished host run left a record: %v", entries)
	}
}

// Under an agent or CI lerd's own group gets the signal, not the run's, so lerd
// has to end the run's group itself before acting on the signal.
func TestRun_hostRunIsKilledWhenLerdIsInterrupted(t *testing.T) {
	stubDir(t)
	sigs := stubSignals(t)
	var raised []os.Signal
	prev := raiseFn
	raiseFn = func(s os.Signal) { raised = append(raised, s) }
	t.Cleanup(func() { raiseFn = prev })
	cmd, pidFile := orphanMaker(t)

	go func() { readPid(t, pidFile); sigs <- syscall.SIGINT }()
	_ = Run(context.Background(), cmd, "")

	if child := readPid(t, pidFile); alive(child) {
		_ = syscall.Kill(child, syscall.SIGKILL)
		t.Fatal("the run's child survived lerd being interrupted")
	}
	if len(raised) != 1 || raised[0] != syscall.SIGINT {
		t.Fatalf("raised %v, want the signal passed back to lerd", raised)
	}
}

// The group is handed the terminal only when lerd holds it: taking it from a
// `lerd pest &` in the background would steal the user's shell.
func TestHostProcAttr_takesTheTerminalOnlyWhenLerdHoldsIt(t *testing.T) {
	if a := hostProcAttr(false); !a.Setpgid || a.Foreground {
		t.Fatalf("attrs = %+v, want an own group left in the background", a)
	}
	if a := hostProcAttr(true); !a.Setpgid || !a.Foreground || a.Ctty != 0 {
		t.Fatalf("attrs = %+v, want an own group in the foreground on stdin", a)
	}
}

// startGroup starts a long command in a process group of its own, standing in
// for what a killed lerd left behind.
func startGroup(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	return cmd.Process.Pid
}

// A lerd killed outright reaps nothing, so the watcher ends the run's group.
func TestSweep_killsTheGroupOfAHostRunWhoseLerdIsGone(t *testing.T) {
	dir := stubDir(t)
	pgid := startGroup(t)
	if _, err := writeRecord(dir, record{Marker: "host", PGID: pgid, PID: deadPid(t)}); err != nil {
		t.Fatal(err)
	}

	Sweep()

	if !waitGone(pgid) {
		t.Fatal("the group of a run whose lerd is gone survived the sweep")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("records left %v", entries)
	}
}

// A group id is free again once its members are gone, so one started after the
// record belongs to someone else and must be left alone.
func TestSweep_leavesAGroupIdTakenAfterTheRunAlone(t *testing.T) {
	dir := stubDir(t)
	pgid := startGroup(t)
	path, err := writeRecord(dir, record{Marker: "host", PGID: pgid, PID: deadPid(t)})
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	Sweep()

	if syscall.Kill(-pgid, 0) != nil {
		t.Fatal("a group started after the record was killed")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("records left %v", entries)
	}
}

// A child started late in the run, outliving a leader that already exited, is
// still the run's: only a live leader newer than the record means reuse.
func TestSweep_killsALateChildWhoseLeaderExited(t *testing.T) {
	dir := stubDir(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	cmd := exec.Command("sh", "-c", "sleep 1; sleep 30 & echo $! > "+pidFile)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pgid := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })
	if _, err := writeRecord(dir, record{Marker: "host", PGID: pgid, PID: deadPid(t)}); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	readPid(t, pidFile)

	Sweep()

	if !waitGone(pgid) {
		t.Fatal("the run's late child survived the sweep")
	}
}

// Without a leader nothing proves the group is still the run's, so after a
// long gap, a watcher that was down, the id may have changed hands: leave it.
func TestSweep_leavesALeaderlessGroupOfAnOldRecordAlone(t *testing.T) {
	dir := stubDir(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	cmd := exec.Command("sh", "-c", "sleep 30 & echo $! > "+pidFile)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pgid := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })
	path, err := writeRecord(dir, record{Marker: "host", PGID: pgid, PID: deadPid(t)})
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	readPid(t, pidFile)

	Sweep()

	if syscall.Kill(-pgid, 0) != nil {
		t.Fatal("a leaderless group behind an old record was killed")
	}
}

// A host run still in progress is its lerd's to end, not the watcher's.
func TestSweep_leavesARunningHostRunAlone(t *testing.T) {
	dir := stubDir(t)
	pgid := startGroup(t)
	if _, err := writeRecord(dir, record{Marker: "host", PGID: pgid, PID: os.Getpid(), Start: startTimeFn(os.Getpid())}); err != nil {
		t.Fatal(err)
	}

	Sweep()

	if syscall.Kill(-pgid, 0) != nil {
		t.Fatal("the group of a run still in progress was killed")
	}
	if len(readRecords(t, dir)) != 1 {
		t.Fatal("the record of a run still in progress was dropped")
	}
}

func deadPid(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func waitGone(pgid int) bool {
	for i := 0; i < 150; i++ {
		if syscall.Kill(-pgid, 0) != nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// Ctrl+Z stops the run's group, not lerd, so lerd has to notice and follow.
func TestProcessStopped_followsStopAndContinue(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	pid := cmd.Process.Pid
	if processStopped(pid) {
		t.Fatal("a running process read as stopped")
	}
	_ = syscall.Kill(pid, syscall.SIGSTOP)
	if !eventually(func() bool { return processStopped(pid) }) {
		t.Fatal("a stopped process did not read as stopped")
	}
	_ = syscall.Kill(pid, syscall.SIGCONT)
	if !eventually(func() bool { return !processStopped(pid) }) {
		t.Fatal("a continued process still read as stopped")
	}
}

func eventually(cond func() bool) bool {
	for i := 0; i < 100; i++ {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
