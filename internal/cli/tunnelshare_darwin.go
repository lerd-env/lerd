package cli

import "syscall"

// Tunnel children get their own process group so stop can kill the whole tree.
// macOS has no Pdeathsig; launchd tears the session down with lerd-ui.
func tunnelSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
