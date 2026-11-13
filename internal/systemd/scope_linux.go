package systemd

import "os/exec"

// scopeLookPath finds systemd-run; a seam for tests.
var scopeLookPath = exec.LookPath

// OwnScope wraps a command so it starts in a systemd scope of its own. A
// service stops by having systemd kill every process in its unit, so a terminal
// lerd-ui or the tray opens would die with them, and lerd update restarts both.
func OwnScope(bin string, args []string) (string, []string) {
	run, err := scopeLookPath("systemd-run")
	if err != nil {
		return bin, args
	}
	return run, append([]string{"--user", "--scope", "--quiet", "--collect", "--", bin}, args...)
}
