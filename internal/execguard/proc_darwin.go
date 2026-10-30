package execguard

import "golang.org/x/sys/unix"

// processStartTime has no cheap source off Linux, so liveness falls back to the
// pid alone.
func processStartTime(int) string { return "" }

// sstop is SSTOP from <sys/proc.h>, which x/sys does not export.
const sstop = 4

func processStopped(pid int) bool {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	return err == nil && kp.Proc.P_stat == sstop
}
