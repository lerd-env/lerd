package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/winshim"
	"github.com/geodro/lerd/internal/wsl"
)

// dumpWSL adds what only a WSL install has: the WSL build, the distro and
// Windows config that shape its networking, and the lerd.exe agent that boots
// the distro and answers DNS for the site TLD.
func dumpWSL(w io.Writer, logLines int) {
	var b bytes.Buffer
	fmt.Fprintln(&b, "── wsl.exe --version")
	cmd := exec.Command(wsl.WindowsTool("wsl.exe"), "--version")
	// UTF-16 otherwise; WSLENV is what carries a variable over to Windows.
	cmd.Env = append(os.Environ(), "WSL_UTF8=1", "WSLENV=WSL_UTF8:"+os.Getenv("WSLENV"))
	if out, err := cmd.Output(); err == nil {
		fmt.Fprintln(&b, strings.TrimSpace(strings.ReplaceAll(string(out), "\r", "")))
	} else {
		fmt.Fprintf(&b, "(failed: %v)\n", err)
	}
	fmt.Fprintln(&b)
	dumpConfigFile(&b, "/etc/wsl.conf", "(no /etc/wsl.conf)")

	profile, _ := windowsEnvPath("USERPROFILE")
	winProfile := ""
	if profile != "" {
		winProfile, _ = windowsPath(profile)
		dumpConfigFile(&b, filepath.Join(profile, ".wslconfig"), "(no .wslconfig)")
	}
	if local, err := windowsEnvPath("LOCALAPPDATA"); err == nil {
		bin := filepath.Join(local, "lerd", "bin")
		dumpConfigFile(&b, filepath.Join(bin, winshim.ConfigName), "(lerd.exe is not set up, run lerd wsl:setup)")
		agentLog := filepath.Join(bin, "lerd-agent.log")
		fmt.Fprintf(&b, "── %s (last %d lines)\n", agentLog, logLines)
		if data, err := os.ReadFile(agentLog); err == nil {
			fmt.Fprintln(&b, tailLines(data, logLines, 500))
		} else {
			fmt.Fprintf(&b, "(unreadable: %v)\n", err)
		}
		fmt.Fprintln(&b)
	}
	fmt.Fprintln(&b, "── Windows NRPT rules from lerd")
	out, err := powershell(`Get-DnsClientNrptRule | Where-Object { $_.Comment -eq 'lerd' } | ForEach-Object { ($_.Namespace -join ',') + ' -> ' + ($_.NameServers -join ',') }`)
	switch {
	case err != nil:
		fmt.Fprintf(&b, "(failed: %v)\n", err)
	case strings.TrimSpace(out) == "":
		fmt.Fprintln(&b, "(none)")
	default:
		fmt.Fprintln(&b, strings.TrimSpace(strings.ReplaceAll(out, "\r", "")))
	}
	home, _ := os.UserHomeDir()
	io.WriteString(w, scrubWindowsProfile(b.String(), profile, winProfile, currentUsername(home))) //nolint:errcheck
}

// scrubWindowsProfile hides the Windows profile, in its /mnt and its C:\
// spelling, and the bare account name when it differs from linuxUser, which
// scrubHomePath already turns into $USER.
func scrubWindowsProfile(s, profile, winProfile, linuxUser string) string {
	var pairs []string
	for _, p := range []string{profile, winProfile} {
		if p != "" {
			pairs = append(pairs, p, "%USERPROFILE%")
		}
	}
	if user := path.Base(profile); profile != "" && len(user) >= 3 && user != linuxUser {
		pairs = append(pairs, user, "%WINUSER%")
	}
	if len(pairs) == 0 {
		return s
	}
	return strings.NewReplacer(pairs...).Replace(s)
}
