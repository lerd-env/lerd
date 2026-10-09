//go:build !windows

package tray

import (
	"os/exec"
	"syscall"
)

func setDetachAttrs(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

func hideWindow(*exec.Cmd) {}
