//go:build linux

package cli

import (
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
