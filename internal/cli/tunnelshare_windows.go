//go:build windows

package cli

import "syscall"

// Tunnel children get their own process group so stop can end the whole tree.
func tunnelSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}
