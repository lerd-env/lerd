//go:build windows

package cli

import (
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
	stillActive           = 259
)

// killPID ends pid and its children. Windows has no signal to hand a windowless
// process, so every signal but the liveness probe (0) is a forced tree kill.
func killPID(pid int, sig syscall.Signal) error {
	if pid < 0 {
		pid = -pid // a negative pid names a group, which is the tree here
	}
	if sig == 0 {
		if processExists(pid) {
			return nil
		}
		return syscall.ESRCH
	}
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

// killProcessGroup ends the whole tree under leaderPID; on Windows a group is
// the tree.
func killProcessGroup(leaderPID int, sig syscall.Signal) error { return killPID(leaderPID, sig) }

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h) //nolint:errcheck
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == stillActive
}

func setOwnProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNewProcessGroup
}

func leadsOwnProcessGroup(cmd *exec.Cmd) bool {
	return cmd.SysProcAttr != nil && cmd.SysProcAttr.CreationFlags&createNewProcessGroup != 0
}

func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess, HideWindow: true}
}
