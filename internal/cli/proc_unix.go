//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// The process layer: signalling, liveness and process groups differ enough
// between Unix and Windows that callers go through these instead of syscall.

// killPID sends sig to pid.
func killPID(pid int, sig syscall.Signal) error { return syscall.Kill(pid, sig) }

// killProcessGroup sends sig to the process group the child leads.
func killProcessGroup(leaderPID int, sig syscall.Signal) error {
	return syscall.Kill(-leaderPID, sig)
}

// processExists reports whether pid is a live process. Signal 0 only checks.
func processExists(pid int) bool { return pid > 0 && syscall.Kill(pid, 0) == nil }

// setOwnProcessGroup makes cmd lead its own group, so what it spawns can be
// reaped with it.
func setOwnProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// leadsOwnProcessGroup reports whether setOwnProcessGroup was applied to cmd.
func leadsOwnProcessGroup(cmd *exec.Cmd) bool {
	return cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid
}

// detachedSysProcAttr starts a child in a session of its own, so it outlives
// the terminal that launched it.
func detachedSysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
