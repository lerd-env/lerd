package execguard

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// groupGrace is how long a group gets to exit on TERM before it is killed; a
// Playwright server closes its browsers on TERM but may not finish in time.
const (
	groupGrace = 2 * time.Second
	groupPoll  = 50 * time.Millisecond
	// guardLife caps how long a guard idles when no watcher runs to sweep it.
	guardLife = 24 * time.Hour
)

var (
	killGroupFn    = killGroup
	ownsTerminalFn = ownsTerminal
)

// runHost runs a command on this machine in a process group of its own and ends
// the group with the run, however the run ends. A marker cannot find the tree
// here: macOS shows no process another's environment.
func runHost(ctx context.Context, cmd *exec.Cmd) error {
	fg := cmd.Stdin == os.Stdin && ownsTerminalFn()
	cmd.SysProcAttr = hostProcAttr(fg)
	sigs, stop := notifyFn()
	defer stop()
	if err := cmd.Start(); err != nil {
		return err
	}
	pgid := cmd.Process.Pid
	guard, _ := startGuard(pgid)
	path, recErr := writeRecord(dirFn(), record{Marker: newMarker(), PGID: pgid, Guard: guard, PID: os.Getpid(), Start: startTimeFn(os.Getpid())})
	// The record is only for a lerd killed outright; every other way out
	// ends the group here and drops it.
	finish := func() {
		if fg {
			reclaimTerminal()
		}
		killGroupFn(pgid)
		if recErr == nil {
			_ = os.Remove(path)
		}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stopped, stopWatch := childStops(fg)
	defer stopWatch()
	for {
		select {
		case err := <-done:
			finish()
			return err
		case <-ctx.Done():
			killGroupFn(pgid)
			err := <-done
			finish()
			return err
		case sig := <-sigs:
			killGroupFn(pgid)
			err := <-done
			finish()
			// The signal was meant for lerd, which still has to act on it.
			stop()
			raiseFn(sig)
			return err
		case <-stopped:
			if processStopped(pgid) {
				suspendWith(pgid)
			}
		}
	}
}

// childStops reports the run's state changes when it holds the terminal, where
// Ctrl+Z stops its group and not lerd.
func childStops(fg bool) (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	if !fg {
		return ch, func() {}
	}
	signal.Notify(ch, syscall.SIGCHLD)
	return ch, func() { signal.Stop(ch) }
}

// suspendWith follows a run Ctrl+Z stopped: lerd takes the terminal back and
// stops too, so the shell sees the job stop, and on fg hands the terminal back
// and continues the run. A bg leaves the terminal with the shell.
func suspendWith(pgid int) {
	reclaimTerminal()
	_ = syscall.Kill(os.Getpid(), syscall.SIGSTOP)
	if ownsTerminalFn() {
		_ = unix.IoctlSetPointerInt(0, unix.TIOCSPGRP, pgid)
	}
	_ = syscall.Kill(-pgid, syscall.SIGCONT)
}

// hostProcAttr puts the run in its own group, handed the terminal when lerd
// holds it so Ctrl+C reaches the run and tinker can read its prompt.
func hostProcAttr(foreground bool) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Foreground: foreground, Ctty: 0}
}

func ownsTerminal() bool {
	if !term.IsTerminal(0) {
		return false
	}
	pgrp, err := unix.IoctlGetInt(0, unix.TIOCGPGRP)
	return err == nil && pgrp == syscall.Getpgrp()
}

// reclaimTerminal takes the terminal back from the run's group, or the next
// prompt lerd shows stops on SIGTTIN. Doing so from the background is what
// raises SIGTTOU, so it is ignored for the call.
func reclaimTerminal() {
	signal.Ignore(syscall.SIGTTOU)
	defer signal.Reset(syscall.SIGTTOU)
	_ = unix.IoctlSetPointerInt(0, unix.TIOCSPGRP, syscall.Getpgrp())
}

// killGroup TERMs a group, then KILLs whatever is left after groupGrace.
func killGroup(pgid int) {
	if syscall.Kill(-pgid, syscall.SIGTERM) != nil {
		return
	}
	for waited := time.Duration(0); waited < groupGrace; waited += groupPoll {
		time.Sleep(groupPoll)
		if syscall.Kill(-pgid, 0) != nil {
			return
		}
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}

// startGuard adds an idle member to the run's group. A group id is only free for
// reuse once every member is gone, so while the guard lives the id is provably
// the run's. It ignores the hangup and Ctrl+C a dying terminal sends, not TERM.
func startGuard(pgid int) (int, error) {
	g := exec.Command("sh", "-c", `trap "" HUP INT; exec sleep `+strconv.Itoa(int(guardLife.Seconds())))
	g.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: pgid}
	if err := g.Start(); err != nil {
		return 0, err
	}
	go func() { _ = g.Wait() }()
	return g.Process.Pid, nil
}

// sweepHost ends the group a lerd killed outright left behind, but only while
// its guard still holds the id; without it the id may belong to someone else.
func sweepHost(r record) {
	if r.Guard <= 0 || syscall.Kill(r.Guard, 0) != nil {
		return
	}
	if pg, err := syscall.Getpgid(r.Guard); err != nil || pg != r.PGID {
		return
	}
	killGroupFn(r.PGID)
}
