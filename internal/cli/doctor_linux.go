//go:build linux

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// PortHolder names what listens on port without asking for sudo, or "" when
// that cannot be told.
func PortHolder(port string) string {
	return portHolder(port, PortListOutput(), func(unit string) bool {
		return exec.Command("systemctl", "is-active", "--quiet", unit+".service").Run() == nil
	})
}

// systemWebServers are the units likely to hold 80/443 as root, where ss
// shows no process to an unprivileged caller, so one running is a hint only.
var systemWebServers = []string{"nginx", "httpd", "apache2", "caddy", "lighttpd"}

func portHolder(port, ssOut string, unitActive func(string) bool) string {
	listening := false
	for _, line := range strings.Split(ssOut, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || !strings.HasSuffix(fields[3], ":"+port) {
			continue
		}
		listening = true
		if name, pid, ok := ssProcess(line); ok {
			return fmt.Sprintf("%s (pid %s)", name, pid)
		}
	}
	if !listening {
		return ""
	}
	for _, unit := range systemWebServers {
		if unitActive(unit) {
			// Running proves nothing about which port it holds, so it is only a guess.
			return "another process, possibly the system " + unit + " service"
		}
	}
	return ""
}

// ssProcess reads the first process out of an ss line's users:(("name",pid=N,...)).
func ssProcess(line string) (name, pid string, ok bool) {
	_, rest, found := strings.Cut(line, `users:(("`)
	if !found {
		return "", "", false
	}
	name, rest, _ = strings.Cut(rest, `"`)
	_, rest, found = strings.Cut(rest, "pid=")
	if !found {
		return "", "", false
	}
	pid, _, _ = strings.Cut(rest, ",")
	return name, pid, name != ""
}

// enforcedMysqldProfiles lists the host's enforced AppArmor profiles for a
// native MySQL or MariaDB server. Rootless podman applies no profile of its
// own, so the host's attaches by path to the mysqld in lerd's container and
// stops it reading its config. root is "/" outside tests.
func enforcedMysqldProfiles(root string) []string {
	enabled, err := os.ReadFile(filepath.Join(root, "sys/module/apparmor/parameters/enabled"))
	if err != nil || strings.TrimSpace(string(enabled)) != "Y" {
		return nil
	}
	var out []string
	for _, name := range []string{"usr.sbin.mysqld", "usr.sbin.mariadbd"} {
		body, err := os.ReadFile(filepath.Join(root, "etc/apparmor.d", name))
		if err != nil || strings.Contains(string(body), "complain") {
			continue
		}
		if _, err := os.Lstat(filepath.Join(root, "etc/apparmor.d/disable", name)); err == nil {
			continue
		}
		out = append(out, "/etc/apparmor.d/"+name)
	}
	return out
}
