package platform

import "os/exec"

var Current = Caps{
	Opener:           "open",
	UsesMachineVM:    true,
	NativePHPRuntime: true,
	WorkerModes:      true,
}

// OpenCommand hands target to the default app through open(1).
func OpenCommand(target string) *exec.Cmd {
	return exec.Command(Current.Opener, target)
}

// LookPath finds a desktop program on PATH.
func LookPath(name string) (string, error) {
	return exec.LookPath(name)
}
