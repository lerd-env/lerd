//go:build windows

package services

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// Windows has no init system to respawn a host process, so a service unit with
// a restart policy runs under `lerd supervise`, which plays systemd's part for
// that one unit: it runs the command, waits for it, and starts it again as the
// policy says. Its pid is the one recorded for the unit, so Stop's tree kill
// ends the supervisor and the process under it together, and nothing respawns.

const (
	superviseMinDelay = time.Second
	superviseMaxDelay = time.Minute
	// superviseHealthyRun is how long a run must last to count as healthy and
	// bring the delay back to superviseMinDelay.
	superviseHealthyRun = time.Minute
)

func (p keepAlivePolicy) String() string {
	switch p {
	case keepAliveAlways:
		return "always"
	case keepAliveOnFailure:
		return "on-failure"
	default:
		return "no"
	}
}

func parseKeepAlivePolicy(s string) (keepAlivePolicy, error) {
	switch s {
	case "always":
		return keepAliveAlways, nil
	case "on-failure":
		return keepAliveOnFailure, nil
	case "no", "":
		return keepAliveNever, nil
	default:
		return keepAliveNever, fmt.Errorf("unknown restart policy %q", s)
	}
}

// shouldRestart applies the policy to an exit code, as systemd's Restart= does:
// on-failure leaves a clean exit alone, so the tray's Quit stays quit.
func shouldRestart(p keepAlivePolicy, exitCode int) bool {
	switch p {
	case keepAliveAlways:
		return true
	case keepAliveOnFailure:
		return exitCode != 0
	default:
		return false
	}
}

// nextDelay is the wait before the next start: doubled after each short run up
// to superviseMaxDelay, so a unit that crashes on start does not spin, and back
// to superviseMinDelay after a healthy run.
func nextDelay(prev, ran time.Duration) time.Duration {
	if prev == 0 || ran >= superviseHealthyRun {
		return superviseMinDelay
	}
	return min(prev*2, superviseMaxDelay)
}

// supervisedArgs wraps a unit's command in `lerd supervise`. Units with no
// restart policy run as they are.
func supervisedArgs(name string, p keepAlivePolicy, args []string) []string {
	if p == keepAliveNever {
		return args
	}
	out := append(supervisorCommand(), "--unit", name, "--restart", p.String(), "--")
	return append(out, args...)
}

// supervisorCommand is the argv prefix that starts a supervisor. Tests point it
// at the test binary, which cannot run `lerd supervise` itself.
var supervisorCommand = func() []string {
	return []string{supervisorBinary(), "supervise"}
}

// supervisorBinary is the installed lerd.exe, so a unit started by a lerd run
// from elsewhere (a build folder during `lerd install`) does not keep that copy
// in use.
func supervisorBinary() string {
	installed := filepath.Join(config.BinDir(), "lerd.exe")
	if _, err := os.Stat(installed); err == nil {
		return installed
	}
	return config.LerdBinary()
}

// Supervise runs args for the named unit until the restart policy says to stop.
// The command inherits the supervisor's output, which spawn points at the
// unit's log, and the supervisor notes there every exit and restart.
func Supervise(unit, restart string, args []string) error {
	return supervise(unit, restart, args, os.Stderr, time.Sleep)
}

func supervise(unit, restart string, args []string, log io.Writer, sleep func(time.Duration)) error {
	if len(args) == 0 {
		return errors.New("lerd supervise: no command to run")
	}
	policy, err := parseKeepAlivePolicy(restart)
	if err != nil {
		return err
	}
	var delay time.Duration
	for {
		started := time.Now()
		code := runOnce(args, log)
		ran := time.Since(started).Round(time.Second)
		if !shouldRestart(policy, code) {
			fmt.Fprintf(log, "lerd supervise: %s exited with code %d, not restarting (restart=%s)\n", unit, code, policy)
			releasePID(unit)
			return nil
		}
		delay = nextDelay(delay, ran)
		fmt.Fprintf(log, "lerd supervise: %s exited with code %d after %s, restarting in %s\n", unit, code, ran, delay)
		sleep(delay)
	}
}

// runOnce runs args to completion and returns its exit code, or -1 when it
// could not be started at all.
func runOnce(args []string, log io.Writer) int {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout, cmd.Stderr = log, log
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// Windows exit codes are unsigned; a killed process reports
		// 0xFFFFFFFF, which reads better as -1.
		return int(int32(exitErr.ExitCode()))
	}
	fmt.Fprintf(log, "lerd supervise: %v\n", err)
	return -1
}

// releasePID drops the unit's pid file when it still names this supervisor, so
// a unit that finished for good reads as inactive rather than failed. A pid
// file that names another process belongs to a newer start and is left alone.
func releasePID(unit string) {
	if unit == "" || readPID(unit) != os.Getpid() {
		return
	}
	config.GuardRealWrite(pidPath(unit))
	_ = os.Remove(pidPath(unit))
}
