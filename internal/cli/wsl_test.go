package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The CA is found in CAROOT, with no mkcert binary anywhere to ask.
func TestMkcertRootCAPEM_FindsTheCA(t *testing.T) {
	caroot := t.TempDir()
	t.Setenv("CAROOT", caroot)
	t.Setenv("PATH", "")
	pem := filepath.Join(caroot, "rootCA.pem")
	if err := os.WriteFile(pem, []byte("pem"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := mkcertRootCAPEM()
	if !ok || got != pem {
		t.Fatalf("mkcertRootCAPEM() = %q, %v; want %q, true", got, ok, pem)
	}
}

func TestMkcertRootCAPEM_NoCAYet(t *testing.T) {
	t.Setenv("CAROOT", t.TempDir())
	if got, ok := mkcertRootCAPEM(); ok {
		t.Fatalf("expected no CA before one is generated, got %q", got)
	}
}

func TestRemoveWindowsSideScript_DropsToastAppID(t *testing.T) {
	got := removeWindowsSideScript(`C:\Users\me\AppData\Local\lerd\bin`)
	if !strings.Contains(got, `Remove-Item -Path 'HKCU:\Software\Classes\AppUserModelId\sh.lerd.Lerd' -Recurse`) {
		t.Errorf("uninstall must drop the toast app id:\n%s", got)
	}
	if !strings.Contains(got, `$d = 'C:\Users\me\AppData\Local\lerd\bin'`) {
		t.Errorf("uninstall must still drop the bin dir from PATH:\n%s", got)
	}
}

func TestWindowsDriveWarning(t *testing.T) {
	got := windowsDriveWarning("/mnt/c/Users/me/app", "/home/me", "Ubuntu-24.04")
	for _, want := range []string{
		"/mnt/c/Users/me/app is on a Windows drive",
		"cp -a /mnt/c/Users/me/app /home/me/app",
		`\\wsl.localhost\Ubuntu-24.04\home\me\app`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("warning lacks %q:\n%s", want, got)
		}
	}
	// Without the distro name the Windows path is left out rather than wrong.
	if got := windowsDriveWarning("/mnt/c/app", "/home/me", ""); strings.Contains(got, "wsl.localhost") {
		t.Errorf("no distro, no UNC path:\n%s", got)
	}
}

// With the desktop app running the agent, an install from before it must not
// keep a Run entry starting a second one.
func TestAgentHandOverScript_DropsTheAgentRunEntry(t *testing.T) {
	got := agentHandOverScript()
	if !strings.Contains(got, `$k = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'`) ||
		!strings.Contains(got, `Remove-ItemProperty -Path $k -Name 'lerd'`) {
		t.Errorf("the hand-over must drop the agent's Run entry:\n%s", got)
	}
	// A fresh install has no entry to drop, and a bare Remove-ItemProperty
	// would fail the script for it, silenced or not.
	if !strings.Contains(got, `if (Get-ItemProperty -Path $k -Name 'lerd' -ErrorAction SilentlyContinue)`) {
		t.Errorf("the hand-over must only drop an entry that is there:\n%s", got)
	}
	if strings.Contains(got, "conhost") || strings.Contains(got, "lerd.exe") {
		t.Errorf("the hand-over must not start the agent itself:\n%s", got)
	}
	// The new config stops the running agent, so a closed app must be started.
	for _, want := range []string{`HKCU:\Software\Classes\lerd\shell\open\command`, `-not (Get-Process -Name lerd-desktop`, `Start-Process $Matches[1] -ArgumentList '--hidden'`} {
		if !strings.Contains(got, want) {
			t.Errorf("the hand-over must start a closed desktop app, missing %s:\n%s", want, got)
		}
	}
}

func TestAgentRunValue_Headless(t *testing.T) {
	got := agentRunValue(`C:\Users\me\AppData\Local\lerd\bin`)
	want := `conhost.exe --headless "C:\Users\me\AppData\Local\lerd\bin\lerd.exe" --agent`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReplaceRunningExe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lerd.exe")

	if err := replaceRunningExe(path, []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".old"); err == nil {
		t.Error("a first install has nothing to move aside")
	}

	// A newer build moves the running one aside rather than overwriting it.
	if err := replaceRunningExe(path, []byte("v2")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "v2" {
		t.Errorf("new exe = %q, want v2", b)
	}
	if b, _ := os.ReadFile(path + ".old"); string(b) != "v1" {
		t.Errorf("old exe = %q, want v1", b)
	}

	// The same build again touches nothing.
	os.Remove(path + ".old")
	if err := replaceRunningExe(path, []byte("v2")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".old"); err == nil {
		t.Error("an unchanged exe should not be moved aside")
	}
}

func TestPinFoldersScript_QuotesPaths(t *testing.T) {
	got := pinFoldersScript([]string{`\\wsl.localhost\Ubuntu\home\o'brien\Lerd`})
	if !strings.Contains(got, `@('\\wsl.localhost\Ubuntu\home\o''brien\Lerd')`) {
		t.Errorf("path not quoted for PowerShell:\n%s", got)
	}
	if !strings.Contains(got, "pintohome") || !strings.Contains(got, quickAccess) {
		t.Errorf("script does not pin to Quick access:\n%s", got)
	}
}

// lerd install rewrites the tray unit and disables it on WSL, so off means not
// enabled and not running; a mask from an older wsl:setup counts too.
func TestTrayUnitOff(t *testing.T) {
	for _, c := range []struct {
		enabled, active string
		want            bool
	}{
		{"disabled", "inactive", true},
		{"masked", "inactive", true},
		{"", "inactive", true},
		{"enabled", "inactive", false},
		{"disabled", "active", false},
		{"disabled", "activating", false},
	} {
		if got := trayUnitOff(c.enabled, c.active); got != c.want {
			t.Errorf("trayUnitOff(%q, %q) = %v, want %v", c.enabled, c.active, got, c.want)
		}
	}
}

func TestMountRoot(t *testing.T) {
	for in, want := range map[string]string{"/mnt/c/": "/mnt", "/mnt/c": "/mnt", "/c/": "/", "": ""} {
		if got := mountRoot(in); got != want {
			t.Errorf("mountRoot(%q) = %q, want %q", in, got, want)
		}
	}
}

// The Windows installer restarts WSL itself right after wsl:setup, so telling
// its user to do it by hand would be wrong; it says so through the env.
func TestWSLSetupDoneNote(t *testing.T) {
	if got := wslSetupDoneNote(false, false); got != "Done. No reboot needed.\n" {
		t.Errorf("nothing to restart: %q", got)
	}
	if got := wslSetupDoneNote(true, false); !strings.Contains(got, "wsl --shutdown") {
		t.Errorf("a person running it is told to restart WSL: %q", got)
	}
	if got := wslSetupDoneNote(true, true); strings.Contains(got, "wsl --shutdown") || !strings.Contains(got, "restarts WSL") {
		t.Errorf("a caller that restarts WSL is not given a manual step: %q", got)
	}
}

// Turning lerd's DNS off must take back its NRPT rule, and only lerd's: a rule
// the user added for another suffix stays.
func TestRemoveNRPTScript_OnlyLerdsRule(t *testing.T) {
	s := removeNRPTScript
	if !strings.Contains(s, "$_.Comment -eq 'lerd'") || !strings.Contains(s, "Remove-DnsClientNrptRule") {
		t.Errorf("does not remove lerd's rule by its comment: %s", s)
	}
	if !strings.Contains(s, "Clear-DnsClientCache") {
		t.Errorf("leaves cached answers for the old suffix: %s", s)
	}
}

// install.ps1 registers the desktop app for the user and has no uninstaller of
// its own, so lerd uninstall stops the app from starting at every login after
// lerd is gone, and takes back its shortcut and lerd:// handler.
func TestRemoveWindowsSideScript_UnregistersTheDesktopApp(t *testing.T) {
	got := removeWindowsSideScript(`C:\x`)
	for _, want := range []string{
		`-Name 'lerd-desktop'`,
		`Remove-Item -Path 'HKCU:\Software\Classes\lerd' -Recurse`,
		`'Lerd.lnk'`,
		`Get-Process -Name lerd-desktop`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s:\n%s", want, got)
		}
	}
}

func TestWindowsSideScriptsQuoteAnApostrophe(t *testing.T) {
	dir := `C:\Users\O'Brien\AppData\Local\lerd\bin`
	for name, script := range map[string]string{
		"agent":   agentScript(dir),
		"path":    addToUserPathScript(dir),
		"removal": removeWindowsSideScript(dir),
	} {
		if !strings.Contains(script, `O''Brien`) || strings.Contains(strings.ReplaceAll(script, `O''Brien`, ""), `O'Brien`) {
			t.Errorf("%s script leaves the apostrophe unescaped:\n%s", name, script)
		}
	}
}

// The WSL VM's Hyper-V firewall drops inbound connections by default. The rule
// must name that VM and the local subnet, and no ports, since lerd's move.
func TestLANFirewallScript(t *testing.T) {
	for _, want := range []string{wslVMCreator, "-RemoteAddresses LocalSubnet", "Get-Command New-NetFirewallHyperVRule"} {
		if !strings.Contains(lanFirewallScript, want) {
			t.Errorf("lanFirewallScript lacks %q", want)
		}
	}
	if strings.Contains(lanFirewallScript, "-LocalPorts") {
		t.Error("a port list breaks as soon as a port moves")
	}
	if !strings.Contains(removeLANFirewallScript, "'lerd-lan'") {
		t.Errorf("the removal must take the rule: %s", removeLANFirewallScript)
	}
}

func TestCertutilResult(t *testing.T) {
	declined := exec.Command("false").Run()
	cases := map[string]struct {
		err  error
		want caResult
	}{
		"imported":         {nil, caTrusted},
		"windows refused":  {declined, caDeclined},
		"certutil missing": {exec.ErrNotFound, caSkippedNoInterop},
	}
	for name, tc := range cases {
		if got := certutilResult(tc.err); got != tc.want {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}
