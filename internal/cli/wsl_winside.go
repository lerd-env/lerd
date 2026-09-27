package cli

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/winshim"
)

// windowsSideTLD is the TLD lerd.exe's agent answers for, empty when lerd does
// not manage DNS: .localhost already resolves to loopback in Windows browsers.
func windowsSideTLD() string {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil || !cfg.DNS.Enabled {
		return ""
	}
	return dns.ConfiguredTLD()
}

// installWindowsSide puts lerd.exe on the Windows PATH, starts its agent at
// every login and now, and routes the site TLD to that agent with an NRPT rule.
// The rule is the one step that needs admin, so it prompts through UAC once.
func installWindowsSide(w io.Writer) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err := os.ReadFile(filepath.Join(filepath.Dir(self), "lerd.exe"))
	if err != nil {
		return fmt.Errorf("lerd.exe is not installed next to %s; reinstall lerd from a release to get it", self)
	}
	distro := os.Getenv("WSL_DISTRO_NAME")
	if distro == "" {
		return fmt.Errorf("WSL_DISTRO_NAME is not set, so lerd.exe would not know which distro to run")
	}
	local, err := windowsEnvPath("LOCALAPPDATA")
	if err != nil {
		return err
	}
	dir := filepath.Join(local, "lerd", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := replaceRunningExe(filepath.Join(dir, "lerd.exe"), exe); err != nil {
		return err
	}
	tld := windowsSideTLD()
	cfg := winshim.Config{Distro: distro, Lerd: self, TLD: tld}
	if err := os.WriteFile(filepath.Join(dir, winshim.ConfigName), []byte(cfg.String()), 0o644); err != nil {
		return err
	}
	winDir, err := windowsPath(dir)
	if err != nil {
		return err
	}
	if _, err := powershell(addToUserPathScript(winDir)); err != nil {
		return fmt.Errorf("adding %s to PATH: %w", winDir, err)
	}
	fmt.Fprintf(w, "  ✓ lerd.exe installed in %s and on your Windows PATH\n", winDir)

	if _, err := powershell(agentScript(winDir)); err != nil {
		return fmt.Errorf("starting the lerd agent: %w", err)
	}
	fmt.Fprintln(w, "  ✓ lerd agent starts at Windows login and is running")

	if tld == "" {
		return nil
	}
	if out, _ := powershell(nrptPresentScript(tld)); strings.TrimSpace(out) == "yes" {
		fmt.Fprintf(w, "  - Windows already sends .%s to lerd\n", tld)
		return nil
	}
	if err := addNRPTRule(w, tld); err != nil {
		return err
	}
	fmt.Fprintf(w, "  ✓ Windows resolves *.%s through lerd\n", tld)
	return nil
}

// replaceRunningExe writes exe to path even while the old one runs: Windows
// refuses to overwrite a running executable but lets it be renamed away.
func replaceRunningExe(path string, exe []byte) error {
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, exe) {
		return nil
	}
	old := path + ".old"
	os.Remove(old) //nolint:errcheck // still running from a previous update; harmless
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, old); err != nil {
			return err
		}
	}
	return os.WriteFile(path, exe, 0o755)
}

// agentRunValue is the HKCU Run entry. conhost --headless keeps the agent from
// opening a console window at every login.
func agentRunValue(winDir string) string {
	return fmt.Sprintf(`conhost.exe --headless "%s\lerd.exe" --agent`, winDir)
}

func agentScript(winDir string) string {
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$k = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
if (-not (Test-Path $k)) { New-Item -Path $k | Out-Null }
Set-ItemProperty -Path $k -Name 'lerd' -Value '%s'
# The config was just rewritten, which makes a running agent exit within
# seconds; wait for it so the new one can take port 53.
for ($i = 0; $i -lt 10; $i++) {
  if (-not (Get-CimInstance Win32_Process -Filter "name='lerd.exe'" | Where-Object { $_.CommandLine -like '*--agent*' })) { break }
  Start-Sleep 1
}
Start-Process conhost.exe -ArgumentList '--headless','"%s\lerd.exe"','--agent' -WindowStyle Hidden`, agentRunValue(winDir), winDir)
}

func addToUserPathScript(winDir string) string {
	return fmt.Sprintf(`$d = '%s'
$p = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($p -split ';') -notcontains $d) {
  [Environment]::SetEnvironmentVariable('Path', (($p.TrimEnd(';'), $d) -join ';').TrimStart(';'), 'User')
}`, winDir)
}

func nrptPresentScript(tld string) string {
	return fmt.Sprintf(`if (Get-DnsClientNrptRule | Where-Object { $_.Namespace -contains '.%s' -and $_.NameServers -contains '127.0.0.1' }) { 'yes' } else { 'no' }`, tld)
}

// addNRPTRule adds the rule sending tld to the agent. Run from an elevated
// process (the installer) it adds and checks in one call; otherwise it goes
// through UAC and waits for the rule, since removing an old one alone can take
// ten seconds.
func addNRPTRule(w io.Writer, tld string) error {
	if out, _ := powershell(isElevatedScript); strings.TrimSpace(out) == "True" {
		out, err := powershell(addNRPTScript(tld) + "\n" + nrptPresentScript(tld))
		if err != nil || strings.TrimSpace(out) != "yes" {
			return fmt.Errorf("adding the Windows DNS rule for .%s failed: %v %s", tld, err, strings.TrimSpace(out))
		}
		return nil
	}
	fmt.Fprintf(w, "  → routing .%s to lerd in Windows DNS, approve the admin prompt on the Windows desktop\n", tld)
	if _, err := powershell(elevated(addNRPTScript(tld))); err != nil {
		return fmt.Errorf("adding the Windows DNS rule for .%s: %w", tld, err)
	}
	var out string
	for i := 0; i < 30; i++ {
		out, _ = powershell(nrptPresentScript(tld))
		if strings.TrimSpace(out) == "yes" {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("the Windows DNS rule for .%s did not appear (was the admin prompt declined?): %s", tld, strings.TrimSpace(out))
}

const isElevatedScript = `([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)`

// addNRPTScript replaces rather than adds, so a rerun after a rule that landed
// late (or a changed TLD) never leaves duplicates behind.
func addNRPTScript(tld string) string {
	return fmt.Sprintf(`Get-DnsClientNrptRule | Where-Object { $_.Comment -eq 'lerd' } | Remove-DnsClientNrptRule -Force
Add-DnsClientNrptRule -Namespace '.%s' -NameServers '127.0.0.1' -Comment 'lerd'
Clear-DnsClientCache`, tld)
}

// elevated wraps script so it runs in an admin PowerShell behind a UAC prompt,
// waiting for it to finish.
func elevated(script string) string {
	return fmt.Sprintf(`Start-Process powershell.exe -Verb RunAs -Wait -WindowStyle Hidden -ArgumentList '-NoProfile','-EncodedCommand','%s'`, encodePowerShell(script))
}

// encodePowerShell renders script for -EncodedCommand (base64 of UTF-16LE),
// which carries it through WSL interop and Windows argument parsing untouched.
func encodePowerShell(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, r := range u {
		b[2*i], b[2*i+1] = byte(r), byte(r>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// powershell runs script and returns its stdout alone: PowerShell reports
// progress ("Preparing modules for first use") as CLIXML on stderr, which
// would otherwise land in the middle of an answer like "yes".
func powershell(script string) (string, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand",
		encodePowerShell("$ProgressPreference = 'SilentlyContinue'\n"+script))
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// windowsEnvPath returns a Windows environment variable's folder as a WSL path.
func windowsEnvPath(name string) (string, error) {
	out, err := powershell("$env:" + name)
	if err != nil {
		return "", fmt.Errorf("powershell.exe interop unavailable: %w", err)
	}
	win := strings.TrimSpace(out)
	if win == "" {
		return "", fmt.Errorf("%%%s%% is empty", name)
	}
	p, err := exec.Command("wslpath", "-u", win).Output()
	if err != nil {
		return "", fmt.Errorf("wslpath: %w", err)
	}
	return strings.TrimSpace(string(p)), nil
}

func windowsPath(p string) (string, error) {
	out, err := exec.Command("wslpath", "-w", p).Output()
	if err != nil {
		return "", fmt.Errorf("wslpath -w %s: %w", p, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// removeWindowsSide undoes installWindowsSide, so no agent is left booting a
// distro with no lerd in it at every login. lerd's CA stays in the Windows
// trust store: removing it raises a dialog a script cannot answer.
func removeWindowsSide() error {
	local, err := windowsEnvPath("LOCALAPPDATA")
	if err != nil {
		return err
	}
	dir := filepath.Join(local, "lerd")
	winBin, err := windowsPath(filepath.Join(dir, "bin"))
	if err != nil {
		return err
	}
	// The agent exits on its own once its config is gone, elevated or not.
	os.Remove(filepath.Join(dir, "bin", winshim.ConfigName)) //nolint:errcheck
	if _, err := powershell(removeWindowsSideScript(winBin)); err != nil {
		return err
	}
	if out, _ := powershell(`if (Get-DnsClientNrptRule | Where-Object { $_.Comment -eq 'lerd' }) { 'yes' }`); strings.TrimSpace(out) == "yes" {
		if _, err := powershell(elevated(`Get-DnsClientNrptRule | Where-Object { $_.Comment -eq 'lerd' } | Remove-DnsClientNrptRule -Force; Clear-DnsClientCache`)); err != nil {
			return fmt.Errorf("removing the Windows DNS rule: %w", err)
		}
	}
	// lerd.exe stays locked for a moment after the agent exits.
	for i := 0; ; i++ {
		err := os.RemoveAll(dir)
		if err == nil || i == 10 {
			return err
		}
		time.Sleep(time.Second)
	}
}

func removeWindowsSideScript(winBin string) string {
	return fmt.Sprintf(`Remove-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'lerd' -ErrorAction SilentlyContinue
$d = '%s'
$p = [Environment]::GetEnvironmentVariable('Path', 'User')
[Environment]::SetEnvironmentVariable('Path', (($p -split ';') | Where-Object { $_ -and $_ -ne $d }) -join ';', 'User')`, winBin)
}
