//go:build windows

package tray

import (
	"os/exec"
	"syscall"
)

const detachedProcess = 0x00000008

func setDetachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess}
}

const createNoWindow = 0x08000000

// hideWindow keeps a console child of the GUI-subsystem tray from flashing a
// console window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
