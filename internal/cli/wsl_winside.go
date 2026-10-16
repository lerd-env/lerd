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
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/desktopnotify"
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/winshim"
	"github.com/geodro/lerd/internal/wsl"
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
	drive, _ := exec.Command("wslpath", "-u", `C:\`).Output()
	cfg := winshim.Config{Distro: distro, Lerd: self, TLD: tld, Shims: config.BinDir(), Mount: mountRoot(strings.TrimSpace(string(drive)))}
	if err := os.WriteFile(filepath.Join(dir, winshim.ConfigName), []byte(cfg.String()), 0o644); err != nil {
		return err
	}
	if err := writeShimExes(dir, windowsShimNames(config.BinDir())); err != nil {
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

	// With the Lerd desktop app registered as the lerd:// handler, the app
	// starts the agent at login, itself started from Program Files. An
	// unsigned exe registered under Run from PowerShell and started through a
	// headless conhost reads to Windows Defender as malware making itself
	// persistent, and it has quarantined lerd.exe for exactly that. The running
	// agent exits on its own now that the config is rewritten, and the app
	// starts it again.
	if desktopnotify.AppInstalled() {
		if _, err := powershell(agentHandOverScript()); err != nil {
			return fmt.Errorf("handing the lerd agent to the desktop app: %w", err)
		}
		fmt.Fprintln(w, "  ✓ the Lerd desktop app runs the lerd agent")
	} else {
		if _, err := powershell(agentScript(winDir)); err != nil {
			return fmt.Errorf("starting the lerd agent: %w", err)
		}
		fmt.Fprintln(w, "  ✓ lerd agent starts at Windows login and is running")
	}

	if dirs := parkedWindowsPaths(); len(dirs) > 0 {
		if _, err := powershell(pinFoldersScript(dirs)); err != nil {
			fmt.Fprintf(w, "  ! pinning your sites folder in Explorer: %v\n", err)
		} else {
			fmt.Fprintln(w, "  ✓ your sites folder is pinned in Explorer's Quick access")
		}
	}

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

// parkedWindowsPaths returns the parked directories that exist, as the
// \\wsl.localhost paths Windows opens them by. Sites live on the distro's own
// disk, which a Windows user otherwise has no obvious way to find.
func parkedWindowsPaths() []string {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return nil
	}
	var out []string
	for _, d := range cfg.ParkedDirectories {
		if _, err := os.Stat(d); err != nil {
			continue
		}
		if p, err := windowsPath(d); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// quickAccess is Explorer's Quick access folder, where pintohome puts a pin.
const quickAccess = `shell:::{679f85cb-0220-4080-b29b-5540cc05aab6}`

func psList(items []string) string {
	quoted := make([]string, len(items))
	for i, it := range items {
		quoted[i] = "'" + strings.ReplaceAll(it, "'", "''") + "'"
	}
	return "@(" + strings.Join(quoted, ",") + ")"
}

// pinFoldersScript pins each folder to Quick access unless it already is: the
// pintohome verb on a pinned folder would do nothing useful.
func pinFoldersScript(dirs []string) string {
	return fmt.Sprintf(`$sh = New-Object -ComObject Shell.Application
$pinned = @($sh.Namespace('%s').Items() | ForEach-Object { $_.Path })
foreach ($d in %s) {
  if ($pinned -notcontains $d) {
    $f = $sh.Namespace($d)
    if ($f) { $f.Self.InvokeVerb('pintohome') }
  }
}`, quickAccess, psList(dirs))
}

// unpinFoldersScript takes those pins back out of Quick access.
func unpinFoldersScript(dirs []string) string {
	return fmt.Sprintf(`$sh = New-Object -ComObject Shell.Application
$sh.Namespace('%s').Items() | Where-Object { %s -contains $_.Path } | ForEach-Object { $_.InvokeVerb('unpinfromhome') }`, quickAccess, psList(dirs))
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

// agentHandOverScript drops the agent's own Run entry, which an install from
// before the desktop app left behind. It checks first: a Remove-ItemProperty
// that finds nothing fails, and even silenced it fails the script.
func agentHandOverScript() string {
	return `$k = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
if (Get-ItemProperty -Path $k -Name 'lerd' -ErrorAction SilentlyContinue) { Remove-ItemProperty -Path $k -Name 'lerd' }`
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
	return fmt.Sprintf(`Start-Process powershell.exe -Verb RunAs -Wait -WindowStyle Hidden -ArgumentList '-NoProfile','-EncodedCommand','%s'`, wsl.EncodePowerShell(script))
}

// powershell runs script and returns its stdout alone: PowerShell reports
// progress ("Preparing modules for first use") as CLIXML on stderr, which
// would otherwise land in the middle of an answer like "yes".
func powershell(script string) (string, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand",
		wsl.EncodePowerShell("$ProgressPreference = 'SilentlyContinue'\n"+script))
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

// windowsShimNames lists the shims in dir that get a .exe twin on Windows: the
// shell scripts lerd writes, not the binaries and version stamps beside them.
func windowsShimNames(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		if e.IsDir() || strings.Contains(e.Name(), ".") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		head := make([]byte, 2)
		n, _ := f.Read(head)
		f.Close()
		if string(head[:n]) == "#!" {
			names = append(names, e.Name())
		}
	}
	return names
}

// writeShimExes puts php.exe and its siblings beside lerd.exe, which runs the
// shim its own name says. They are hard links to lerd.exe, or copies where the
// disk refuses one, and a .cmd would not do: cmd.exe will not start in a
// \\wsl.localhost folder and falls back to C:\Windows. Shims lerd no longer
// has are dropped, and a running one is renamed away as replaceRunningExe does.
func writeShimExes(dir string, names []string) error {
	lerdExe := filepath.Join(dir, "lerd.exe")
	stale, _ := filepath.Glob(filepath.Join(dir, "*.cmd"))
	exes, _ := filepath.Glob(filepath.Join(dir, "*.exe"))
	for _, f := range exes {
		if f != lerdExe {
			stale = append(stale, f)
		}
	}
	for _, f := range stale {
		if os.Remove(f) != nil {
			os.Remove(f + ".old")  //nolint:errcheck
			os.Rename(f, f+".old") //nolint:errcheck // still running; the next setup clears it
		}
	}
	for _, name := range names {
		dst := filepath.Join(dir, name+".exe")
		if os.Link(lerdExe, dst) == nil {
			continue
		}
		b, err := os.ReadFile(lerdExe)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// mountRoot is where WSL mounts Windows drives, given wslpath's answer for C:\.
func mountRoot(cDrive string) string {
	if cDrive == "" {
		return ""
	}
	return path.Dir(strings.TrimSuffix(cDrive, "/"))
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
	if dirs := parkedWindowsPaths(); len(dirs) > 0 {
		powershell(unpinFoldersScript(dirs)) //nolint:errcheck // a leftover pin is harmless
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
Remove-Item -Path 'HKCU:\Software\Classes\AppUserModelId\%s' -Recurse -ErrorAction SilentlyContinue
$d = '%s'
$p = [Environment]::GetEnvironmentVariable('Path', 'User')
[Environment]::SetEnvironmentVariable('Path', (($p -split ';') | Where-Object { $_ -and $_ -ne $d }) -join ';', 'User')`, desktopnotify.ToastAppID, winBin)
}
