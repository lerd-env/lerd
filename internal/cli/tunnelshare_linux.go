package cli

import "syscall"

// Tunnel children get their own process group so stop can kill the whole tree,
// and Pdeathsig ties them to lerd-ui when it isn't supervised by systemd
// (which kills the control group itself).
func tunnelSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
}
