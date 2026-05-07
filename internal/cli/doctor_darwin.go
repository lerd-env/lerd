//go:build darwin

package cli

import (
	"os/exec"
	"strings"
)

// PortListOutput returns a summary of listening TCP ports for batch checks.
func PortListOutput() string {
	out, err := exec.Command("lsof", "-nP", "-iTCP", "-sTCP:LISTEN").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// PortInUse returns true if something is listening on the given TCP port.
func PortInUse(port string) bool {
	out, err := exec.Command("lsof", "-nP", "-iTCP:"+port, "-sTCP:LISTEN").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), ":"+port)
}

// FindListenerCmd returns the platform-appropriate shell command the user
// can run to identify the process bound to the given TCP port. macOS lacks
// ss(8) (iproute2), so we point users at lsof which is shipped with the OS.
func FindListenerCmd(port string) string {
	return "lsof -nP -iTCP:" + port + " -sTCP:LISTEN"
}
