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

func guardOf(t *testing.T, pgid int) int {
	t.Helper()
	pid, err := startGuard(pgid)
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

// A lerd killed outright reaps nothing, so the watcher ends the run's group.
func TestSweep_killsTheGroupOfAHostRunWhoseLerdIsGone(t *testing.T) {
	dir := stubDir(t)
	pgid := startGroup(t)
	if _, err := writeRecord(dir, record{Marker: "host", PGID: pgid, Guard: guardOf(t, pgid), PID: deadPid(t)}); err != nil {
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

// The guard holds the group id while it lives, so a child the run started late,
// after its leader exited, is still provably the run's.
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
	if _, err := writeRecord(dir, record{Marker: "host", PGID: pgid, Guard: guardOf(t, pgid), PID: deadPid(t)}); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	readPid(t, pidFile)

	Sweep()

	if !waitGone(pgid) {
		t.Fatal("the run's late child survived the sweep")
	}
}

// Without its guard nothing proves the id is still the run's: once every member
// is gone the id can be reused, so the group is left alone and the record dropped.
func TestSweep_leavesAGroupWithoutItsGuardAlone(t *testing.T) {
	for name, guard := range map[string]func(t *testing.T) int{
		"guard gone":             deadPid,
		"guard in another group": func(*testing.T) int { return os.Getpid() },
	} {
		t.Run(name, func(t *testing.T) {
			dir := stubDir(t)
			pgid := startGroup(t)
			if _, err := writeRecord(dir, record{Marker: "host", PGID: pgid, Guard: guard(t), PID: deadPid(t)}); err != nil {
				t.Fatal(err)
			}

			Sweep()

			if syscall.Kill(-pgid, 0) != nil {
				t.Fatal("a group whose guard is not in it was killed")
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Fatalf("records left %v", entries)
			}
		})
	}
}

// A host run still in progress is its lerd's to end, not the watcher's.
func TestSweep_leavesARunningHostRunAlone(t *testing.T) {
	dir := stubDir(t)
	pgid := startGroup(t)
	if _, err := writeRecord(dir, record{Marker: "host", PGID: pgid, Guard: guardOf(t, pgid), PID: os.Getpid(), Start: startTimeFn(os.Getpid())}); err != nil {
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

// A lerd killed in a terminal takes the terminal with it, and the hangup reaches
// the run's group: the guard has to outlast it, or the sweep cannot prove the id.
func TestStartGuard_outlastsAHangupButNotTerm(t *testing.T) {
	pgid := startGroup(t)
	guard := guardOf(t, pgid)
	time.Sleep(200 * time.Millisecond) // let the guard set its dispositions
	_ = syscall.Kill(-pgid, syscall.SIGHUP)
	_ = syscall.Kill(-pgid, syscall.SIGINT)
	time.Sleep(200 * time.Millisecond)
	if !alive(guard) {
		t.Fatal("the guard died on a hangup")
	}
	killGroup(pgid)
	if !waitGone(pgid) {
		t.Fatal("the guard survived the group being killed")
	}
}

// A run can last longer than any one nap, queue:work for days, and the guard has
// to hold the id for all of it.
func TestStartGuard_outlivesItsNap(t *testing.T) {
	prev := guardNap
	guardNap = time.Second
	t.Cleanup(func() { guardNap = prev })
	pgid := startGroup(t)
	guard := guardOf(t, pgid)
	time.Sleep(2500 * time.Millisecond)
	if !alive(guard) {
		t.Fatal("the guard ended while its run was still going")
	}
}

// SIGSTOP to itself does not stop lerd before its next line runs, so without
// waiting for the shell's fg, lerd resumed the run as soon as Ctrl+Z stopped it.
func TestSuspendWith_resumesTheRunOnlyAfterLerdIsContinued(t *testing.T) {
	prevStop, prevCont, prevOwns := stopSelfFn, continueGroupFn, ownsTerminalFn
	stopSelfFn = func() {} // a stop that has not taken effect yet
	resumed := make(chan int, 1)
	continueGroupFn = func(pgid int) { resumed <- pgid }
	ownsTerminalFn = func() bool { return false }
	t.Cleanup(func() { stopSelfFn, continueGroupFn, ownsTerminalFn = prevStop, prevCont, prevOwns })

	go suspendWith(4242)
	select {
	case <-resumed:
		t.Fatal("the run was resumed before lerd was continued")
	case <-time.After(300 * time.Millisecond):
	}
	_ = syscall.Kill(os.Getpid(), syscall.SIGCONT) // what the shell's fg sends
	select {
	case pgid := <-resumed:
		if pgid != 4242 {
			t.Fatalf("resumed group %d, want 4242", pgid)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fg did not resume the run")
	}
}

// `pnpm exec vite build` or `composer exec` on this machine looks like a podman
// exec to an argv check, and stamping a marker into it breaks the command.
func TestRun_aHostCommandWithExecInItsArgsIsLeftAlone(t *testing.T) {
	stubDir(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "exec"), []byte("exit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, container := range map[string]string{"no container": "", "a container but not podman": "lerd-php85-fpm"} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("sh", "exec")
			cmd.Dir = dir
			if err := Run(context.Background(), cmd, container); err != nil {
				t.Fatal(err)
			}
			if len(cmd.Args) != 2 {
				t.Fatalf("args = %v, want them untouched", cmd.Args)
			}
		})
	}
}
