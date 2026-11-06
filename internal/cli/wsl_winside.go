package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
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
		return fmt.Errorf("lerd.exe is not installed next to %s; run lerd update to fetch it", self)
	}
	// Distro falls back from WSL_DISTRO_NAME, which an SSH or systemd session lacks.
	distro := wsl.Distro()
	if distro == "" {
		return fmt.Errorf("could not tell which distro this is, so lerd.exe would not know which one to run")
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
	if u, err := user.Current(); err == nil {
		cfg.User = u.Username
	}
	if err := os.WriteFile(filepath.Join(dir, winshim.ConfigName), []byte(cfg.String()), 0o644); err != nil {
		return err
	}
	if err := syncShimTwins(dir); err != nil {
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

	// The desktop app starts the agent when installed: an unsigned exe put under
	// Run from PowerShell reads to Defender as malware making itself persistent,
	// and it quarantined lerd.exe for that. The old agent exits on the new config.
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

	// The admin steps share one prompt: the DNS rule and the LAN firewall rule.
	var scripts, reasons []string
	addDNS := false
	switch {
	case tld == "":
		if err := removeNRPTRule(w); err != nil {
			return err
		}
	case nrptPresent(tld):
		fmt.Fprintf(w, "  - Windows already sends .%s to lerd\n", tld)
	default:
		addDNS = true
		scripts = append(scripts, addNRPTScript(tld))
		reasons = append(reasons, "routing ."+tld+" to lerd in Windows DNS")
	}
	addFirewall := !lanFirewallPresent()
	if addFirewall {
		scripts = append(scripts, lanFirewallScript)
		reasons = append(reasons, "letting the LAN reach lerd's shares")
	}
	if len(scripts) == 0 {
		return nil
	}
	if err := runAsAdmin(w, strings.Join(reasons, " and "), strings.Join(scripts, "\n")); err != nil {
		return err
	}
	if addDNS {
		if err := waitForNRPT(tld); err != nil {
			return err
		}
		fmt.Fprintf(w, "  ✓ Windows resolves *.%s through lerd\n", tld)
	}
	if addFirewall {
		if !lanFirewallPresent() {
			return fmt.Errorf("the Windows firewall rule for lerd's LAN shares did not appear (was the admin prompt declined?)")
		}
		fmt.Fprintln(w, "  ✓ the LAN can reach lerd's shares through the WSL firewall")
	}
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

// psQuote is s as a PowerShell single-quoted literal, so a profile folder
// such as C:\Users\O'Brien cannot end the string early.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func psList(items []string) string {
	quoted := make([]string, len(items))
	for i, it := range items {
		quoted[i] = psQuote(it)
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

// syncShimTwins gives each of lerd's shims a .exe twin in dir, the Windows
// side's bin folder.
func syncShimTwins(dir string) error {
	names, err := winshim.ShimNames(config.BinDir())
	if err != nil {
		return err
	}
	return winshim.SyncTwins(dir, names)
}

// refreshWindowsSide brings a Windows side wsl:setup installed up to this lerd,
// for an update or a reinstall: its lerd.exe and the shims' twins. The config
// stays, since rewriting it stops the agent and only wsl:setup starts it again.
func refreshWindowsSide() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err := os.ReadFile(filepath.Join(filepath.Dir(self), "lerd.exe"))
	if err != nil {
		return nil // a build without its Windows side; wsl:setup says so
	}
	local, err := windowsEnvPath("LOCALAPPDATA")
	if err != nil {
		return err
	}
	dir := filepath.Join(local, "lerd", "bin")
	if _, err := os.Stat(filepath.Join(dir, winshim.ConfigName)); err != nil {
		return nil // never set up
	}
	if err := replaceRunningExe(filepath.Join(dir, "lerd.exe"), exe); err != nil {
		return err
	}
	return syncShimTwins(dir)
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

// agentHandOverScript drops the agent's Run entry an older install left, if any
// (a Remove-ItemProperty that finds nothing fails the script). The new config
// stops the running agent, so a closed desktop app is started to run the next.
func agentHandOverScript() string {
	return `$k = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
if (Get-ItemProperty -Path $k -Name 'lerd' -ErrorAction SilentlyContinue) { Remove-ItemProperty -Path $k -Name 'lerd' }
$c = (Get-ItemProperty -Path 'HKCU:\Software\Classes\lerd\shell\open\command' -ErrorAction SilentlyContinue).'(default)'
if ($c -match '^"([^"]+)"' -and -not (Get-Process -Name lerd-desktop -ErrorAction SilentlyContinue)) { Start-Process $Matches[1] -ArgumentList '--hidden' }`
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
Set-ItemProperty -Path $k -Name 'lerd' -Value %s
# The config was just rewritten, which makes a running agent exit within
# seconds; wait for it so the new one can take port 53.
for ($i = 0; $i -lt 10; $i++) {
  if (-not (Get-CimInstance Win32_Process -Filter "name='lerd.exe'" | Where-Object { $_.CommandLine -like '*--agent*' })) { break }
  Start-Sleep 1
}
Start-Process conhost.exe -ArgumentList '--headless',%s,'--agent' -WindowStyle Hidden`, psQuote(agentRunValue(winDir)), psQuote(`"`+winDir+`\lerd.exe"`))
}

func addToUserPathScript(winDir string) string {
	return fmt.Sprintf(`$d = %s
$p = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($p -split ';') -notcontains $d) {
  [Environment]::SetEnvironmentVariable('Path', (($p.TrimEnd(';'), $d) -join ';').TrimStart(';'), 'User')
}`, psQuote(winDir))
}

func nrptPresentScript(tld string) string {
	return fmt.Sprintf(`if (Get-DnsClientNrptRule | Where-Object { $_.Namespace -contains '.%s' -and $_.NameServers -contains '127.0.0.1' }) { 'yes' } else { 'no' }`, tld)
}

func nrptPresent(tld string) bool {
	out, _ := powershell(nrptPresentScript(tld))
	return strings.TrimSpace(out) == "yes"
}

// runAsAdmin runs script directly when already elevated, otherwise behind a
// UAC prompt it waits on, saying what the prompt is for.
func runAsAdmin(w io.Writer, why, script string) error {
	if out, _ := powershell(isElevatedScript); strings.TrimSpace(out) != "True" {
		fmt.Fprintf(w, "  → %s, approve the admin prompt on the Windows desktop\n", why)
		script = elevated(script)
	}
	if _, err := powershell(script); err != nil {
		return fmt.Errorf("%s: %w", why, err)
	}
	return nil
}

// waitForNRPT waits for the rule, since removing an old one alone can take ten
// seconds after the elevated script returned.
func waitForNRPT(tld string) error {
	for i := 0; i < 30; i++ {
		if nrptPresent(tld) {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("the Windows DNS rule for .%s did not appear (was the admin prompt declined?)", tld)
}

// wslVMCreator is the id Windows gives WSL's VM in its Hyper-V firewall.
const wslVMCreator = `{40E0AC32-46A5-438A-A0B2-2B479E8F2E90}`

// lanFirewallScript lets the local subnet through WSL's Hyper-V firewall, which
// blocks all inbound by default. No ports, since lerd's move; only a LAN feature
// listens beyond loopback anyway. Windows without that firewall has none to open.
const lanFirewallScript = `if (Get-Command New-NetFirewallHyperVRule -ErrorAction SilentlyContinue) {
  Remove-NetFirewallHyperVRule -Name 'lerd-lan' -ErrorAction SilentlyContinue
  New-NetFirewallHyperVRule -Name 'lerd-lan' -DisplayName 'lerd LAN' -Direction Inbound -VMCreatorId '` + wslVMCreator + `' -RemoteAddresses LocalSubnet -Action Allow | Out-Null
}`

const removeLANFirewallScript = `Remove-NetFirewallHyperVRule -Name 'lerd-lan' -ErrorAction SilentlyContinue`

// lanFirewallPresent also says yes on a Windows without the Hyper-V firewall,
// where there is no rule to add.
func lanFirewallPresent() bool {
	out, _ := powershell(`if (-not (Get-Command Get-NetFirewallHyperVRule -ErrorAction SilentlyContinue) -or (Get-NetFirewallHyperVRule -Name 'lerd-lan' -ErrorAction SilentlyContinue)) { 'yes' }`)
	return strings.TrimSpace(out) == "yes"
}

const isElevatedScript = `([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)`

// removeNRPTScript takes back lerd's rule, found by its comment, and the
// answers Windows cached through it.
const removeNRPTScript = `Get-DnsClientNrptRule | Where-Object { $_.Comment -eq 'lerd' } | Remove-DnsClientNrptRule -Force; Clear-DnsClientCache`

func lerdNRPTRulePresent() bool {
	out, _ := powershell(`if (Get-DnsClientNrptRule | Where-Object { $_.Comment -eq 'lerd' }) { 'yes' }`)
	return strings.TrimSpace(out) == "yes"
}

// removeNRPTRule drops lerd's rule once lerd no longer answers DNS for a TLD,
// or Windows keeps sending the old suffix to an agent that stopped answering.
func removeNRPTRule(w io.Writer) error {
	if !lerdNRPTRulePresent() {
		return nil
	}
	script := removeNRPTScript
	if out, _ := powershell(isElevatedScript); strings.TrimSpace(out) != "True" {
		fmt.Fprintln(w, "  → removing lerd's Windows DNS rule, approve the admin prompt on the Windows desktop")
		script = elevated(script)
	}
	if _, err := powershell(script); err != nil {
		return fmt.Errorf("removing the Windows DNS rule: %w", err)
	}
	fmt.Fprintln(w, "  ✓ Windows no longer sends a site TLD to lerd")
	return nil
}

// addNRPTScript replaces rather than adds, so a rerun after a rule that landed
// late, or a changed TLD, never leaves duplicates behind.
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
	// By its path: a systemd unit or an SSH session has no Windows PATH.
	ps := wsl.PowerShell()
	if ps == "" {
		return "", fmt.Errorf("powershell.exe not found")
	}
	var stderr bytes.Buffer
	cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-EncodedCommand",
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

// removeWindowsSide undoes installWindowsSide and the desktop app's sign-in
// entry, shortcut and lerd:// handler, so nothing starts lerd after it is gone.
// The CA stays trusted: removing it raises a dialog a script cannot answer.
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
	// One admin prompt takes back both rules.
	var admin []string
	if lerdNRPTRulePresent() {
		admin = append(admin, removeNRPTScript)
	}
	if out, _ := powershell(`if (Get-Command Get-NetFirewallHyperVRule -ErrorAction SilentlyContinue) { if (Get-NetFirewallHyperVRule -Name 'lerd-lan' -ErrorAction SilentlyContinue) { 'yes' } }`); strings.TrimSpace(out) == "yes" {
		admin = append(admin, removeLANFirewallScript)
	}
	if len(admin) > 0 {
		if _, err := powershell(elevated(strings.Join(admin, "\n"))); err != nil {
			return fmt.Errorf("removing lerd's Windows DNS and firewall rules: %w", err)
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
Remove-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'lerd-desktop' -ErrorAction SilentlyContinue
Get-Process -Name lerd-desktop -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Remove-Item -Path 'HKCU:\Software\Classes\lerd' -Recurse -ErrorAction SilentlyContinue
Remove-Item -Path (Join-Path ([Environment]::GetFolderPath('Programs')) 'Lerd.lnk') -ErrorAction SilentlyContinue
Remove-Item -Path 'HKCU:\Software\Classes\AppUserModelId\%s' -Recurse -ErrorAction SilentlyContinue
$d = %s
$p = [Environment]::GetEnvironmentVariable('Path', 'User')
[Environment]::SetEnvironmentVariable('Path', (($p -split ';') | Where-Object { $_ -and $_ -ne $d }) -join ';', 'User')`, desktopnotify.ToastAppID, psQuote(winBin))
}
