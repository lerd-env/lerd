//go:build linux

package cli

import (
	"os/exec"
	"strings"
)

// PortListOutput returns the raw output of ss -tlnp for batch port checks.
func PortListOutput() string {
	out, err := exec.Command("ss", "-tlnp").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// PortInUse returns true if something is listening on the given TCP port.
func PortInUse(port string) bool {
	return strings.Contains(PortListOutput(), ":"+port+" ")
}

// FindListenerCmd returns the platform-appropriate shell command the user
// can run to identify the process bound to the given TCP port. The CLI/UI
// surfaces it in conflict hints so users don't have to know that ss is
// Linux-only and lsof is the macOS equivalent.
func FindListenerCmd(port string) string {
	return "ss -tlnp sport = :" + port
}
