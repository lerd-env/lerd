//go:build windows

package cli

import (
	"os/exec"
	"strings"
)

// PortListOutput returns the listening TCP sockets, one per line, for batch
// checks. Only listeners: callers search the text for ":<port>", which on the
// full connection table would match the remote end of any ordinary connection.
func PortListOutput() string {
	out, err := exec.Command("netstat", "-ano", "-p", "TCP").Output()
	if err != nil {
		return ""
	}
	return listeningOnly(string(out))
}

// listeningOnly keeps the LISTENING rows of netstat output.
func listeningOnly(out string) string {
	var sb strings.Builder
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 4 && strings.EqualFold(f[0], "TCP") && strings.EqualFold(f[len(f)-2], "LISTENING") {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// PortListOutput returns the listening TCP sockets for batch checks.

// PortInUse returns true if something is listening on the given TCP port.
func PortInUse(port string) bool {
	return listeningOnPort(PortListOutput(), port)
}

// listeningOnPort scans netstat output for a LISTENING socket on port, matching
// the local address column exactly so :80 does not match :8080.
func listeningOnPort(out, port string) bool {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || !strings.EqualFold(f[0], "TCP") || !strings.EqualFold(f[len(f)-2], "LISTENING") {
			continue
		}
		if i := strings.LastIndex(f[1], ":"); i >= 0 && f[1][i+1:] == port {
			return true
		}
	}
	return false
}

// FindListenerCmd is the PowerShell command a user can run to name the process
// bound to port.
func FindListenerCmd(port string) string {
	return "Get-Process -Id (Get-NetTCPConnection -LocalPort " + port + " -State Listen).OwningProcess"
}

// enforcedMysqldProfiles is AppArmor's, which Windows does not have.
func enforcedMysqldProfiles(string) []string { return nil }
