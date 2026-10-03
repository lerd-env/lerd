//go:build windows

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/geodro/lerd/internal/p9share"
	"github.com/geodro/lerd/internal/podman"
)

// p9Guard keeps lerd's 9p server up for as long as the machine runs. When the
// server dies the shares do not come back with a new one: each VM mount is tied
// to the connection that died, and every container bind-mounts the mount it
// started on. So after starting the server again the guard remounts the shares
// in the VM and restarts lerd's containers onto the new mounts.
type p9Guard struct {
	// start launches the server and returns a wait that blocks until it exits
	// and gives its exit code.
	start func() (wait func() int, err error)
	// machineUp reports whether the machine's network process still runs; the
	// server exits cleanly when it goes, and so does the guard.
	machineUp func() bool
	// recover remounts the shares and restarts the containers.
	recover func() error
	sleep   func(time.Duration)
	log     io.Writer
}

const (
	p9GuardMinDelay = time.Second
	p9GuardMaxDelay = 30 * time.Second
)

func (g p9Guard) run() {
	var delay time.Duration
	for restarted := false; ; restarted = true {
		wait, err := g.start()
		if err != nil {
			fmt.Fprintf(g.log, "lerd p9-guard: starting the 9p server: %v\n", err)
		} else {
			if restarted {
				if err := g.recover(); err != nil {
					fmt.Fprintf(g.log, "lerd p9-guard: bringing the shares back: %v\n", err)
				} else {
					fmt.Fprintln(g.log, "lerd p9-guard: shares remounted and containers restarted")
				}
			}
			code := wait()
			if code == 0 || !g.machineUp() {
				// Serve returns cleanly only once the machine is gone.
				return
			}
			fmt.Fprintf(g.log, "lerd p9-guard: the 9p server exited with code %d\n", code)
		}
		if !g.machineUp() {
			return
		}
		if delay == 0 {
			delay = p9GuardMinDelay
		} else {
			delay = min(delay*2, p9GuardMaxDelay)
		}
		fmt.Fprintf(g.log, "lerd p9-guard: starting it again in %s\n", delay)
		g.sleep(delay)
	}
}

// runP9Guard is `lerd p9-guard`: serverArgs are the `lerd p9-serve` arguments,
// that is Podman's server9p ones.
func runP9Guard(machine string, serverArgs []string) error {
	shares, pid, err := p9share.ParseServerArgs(serverArgs)
	if err != nil {
		return err
	}
	plan, err := remountPlan(machine, shares)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	p9Guard{
		start: func() (func() int, error) {
			cmd := exec.Command(self, append([]string{"p9-serve"}, serverArgs...)...)
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			// The guard runs detached with no console; give the server a hidden
			// one rather than let Windows open a window for it.
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000 /* CREATE_NO_WINDOW */, HideWindow: true}
			if err := cmd.Start(); err != nil {
				return nil, err
			}
			return func() int { return exitCode(cmd.Wait()) }, nil
		},
		machineUp: func() bool { return processExists(pid) },
		recover:   func() error { return recoverP9Shares(machine, plan) },
		sleep:     time.Sleep,
		log:       os.Stderr,
	}.run()
	return nil
}

// recoverP9Shares points the VM and the containers at a restarted server: the
// old mounts are lazily detached, the shares mounted again, and the running
// lerd containers restarted so their bind mounts resolve to the new mounts.
func recoverP9Shares(machine string, plan []p9share.Mount) error {
	if err := vmRun(machine, p9share.UnmountScript(plan)); err != nil {
		return fmt.Errorf("unmounting: %w", err)
	}
	if err := remount(machine, plan); err != nil {
		return fmt.Errorf("mounting: %w", err)
	}
	out, err := podman.Cmd("ps", "-q", "--filter", "name=^lerd-").Output()
	if err != nil {
		return fmt.Errorf("listing containers: %w", err)
	}
	if ids := strings.Fields(string(out)); len(ids) > 0 {
		if err := podman.Cmd(append([]string{"restart", "-t", "5"}, ids...)...).Run(); err != nil {
			return fmt.Errorf("restarting containers: %w", err)
		}
	}
	return nil
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exitErr):
		return int(int32(exitErr.ExitCode()))
	default:
		return -1
	}
}
