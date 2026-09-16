package cli

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/term"
)

// isInteractiveStdin reports whether stdin is a terminal, which decides whether
// a native run may be put in its own process group at all. It has to be a real
// terminal check: /dev/null is a character device too, so a mode test calls a
// redirected run interactive and skips the grouping it needs.
func isInteractiveStdin() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// groupNativeRun puts a non-interactive native run in its own process group and
// returns whether it did. An interactive run is left alone: a background process
// group cannot read the terminal (it takes SIGTTIN) and would lose Ctrl-C, which
// tinker and the console commands depend on.
func groupNativeRun(cmd *exec.Cmd) bool {
	if isInteractiveStdin() {
		return false
	}
	setOwnProcessGroup(cmd)
	return true
}

// setOwnProcessGroup puts a native run in its own process group so anything it
// spawns can be reaped with it. Without this a child that outlives the command
// (pest-plugin-browser's Playwright server is the one that bites) keeps the
// inherited stdout open, so a pipeline never sees EOF, and every run leaks one.
func setOwnProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// reapProcessGroup signals whatever is left of a finished command's group. The
// command itself has already exited; this is only for what it left behind.
func reapProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	// Only a group this package created may be signalled, and Setpgid makes the
	// group id the child's pid. Looking it up instead would fail here: Wait has
	// already reaped the process the id would be read from.
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}
