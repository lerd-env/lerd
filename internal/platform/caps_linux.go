package platform

import (
	"os/exec"

	"github.com/geodro/lerd/internal/wsl"
)

var Current = Caps{
	Opener: "xdg-open",
}

// OpenCommand hands target to the desktop's default app, nil when there is no
// opener. Inside WSL that desktop is Windows, reached through interop.
func OpenCommand(target string) *exec.Cmd {
	if wsl.IsWSL() {
		return wsl.OpenCommand(target)
	}
	p, err := exec.LookPath(Current.Opener)
	if err != nil {
		return nil
	}
	return exec.Command(p, target)
}

// LookPath finds a desktop program. Inside WSL that includes the Windows PATH,
// which a systemd service such as lerd-ui does not inherit.
func LookPath(name string) (string, error) {
	if wsl.IsWSL() {
		return wsl.LookPath(name)
	}
	return exec.LookPath(name)
}
