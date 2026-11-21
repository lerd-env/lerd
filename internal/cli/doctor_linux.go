//go:build linux

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
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

// OtherUserLerd names the user whose lerd holds one of the ports lerd needs, or
// "" when none does. lerd is rootless per user, so a second user's stack cannot
// start while the first one's is up, and a lingering stack survives logout.
func OtherUserLerd() string {
	out, err := exec.Command("ss", "-tlne").Output()
	if err != nil {
		return ""
	}
	httpPort, httpsPort := config.NginxPorts()
	ports := []string{strconv.Itoa(httpPort), strconv.Itoa(httpsPort), "5300", "7073"}
	uid := otherUserLerdUID(string(out), ports, strconv.Itoa(os.Getuid()))
	if uid == "" {
		return ""
	}
	if u, err := user.LookupId(uid); err == nil {
		return u.Username
	}
	return "uid " + uid
}

// otherUserLerdUID returns the owner uid of the first listener on ports that
// runs in a lerd unit of a user other than selfUID. ss -e shows the owner and
// cgroup of every socket, including ones the caller cannot see a process for.
func otherUserLerdUID(ssOut string, ports []string, selfUID string) string {
	for _, line := range strings.Split(ssOut, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || !strings.Contains(line, "/app.slice/lerd-") {
			continue
		}
		uid := ""
		for _, f := range fields {
			if v, found := strings.CutPrefix(f, "uid:"); found {
				uid = v
			}
		}
		if uid == "" || uid == selfUID {
			continue
		}
		for _, port := range ports {
			if strings.HasSuffix(fields[3], ":"+port) {
				return uid
			}
		}
	}
	return ""
}
