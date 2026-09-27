package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/winshim"
)

// setupDistro is the distro the installer creates when none is set up for lerd.
const setupDistro = "Ubuntu-24.04"

var stdin = bufio.NewReader(os.Stdin)

// errHandedOff means an elevated copy took over, so this window just closes.
var errHandedOff = errors.New("handed off to the elevated setup")

// setup is lerd.exe's installer mode: it takes a Windows machine with nothing on
// it to a running lerd, one step at a time. Every step checks before it acts,
// so a rerun, or the resume after the reboot WSL needs, carries on where the
// last run stopped.
func setup() error {
	if !isAdmin() {
		fmt.Println("Lerd setup needs administrator rights to install WSL; approve the prompt.")
		if err := run("powershell.exe", "-NoProfile", "-Command",
			fmt.Sprintf(`Start-Process -FilePath '%s' -ArgumentList '--setup' -Verb RunAs`, selfPath())); err != nil {
			return err
		}
		return errHandedOff
	}
	disableQuickEdit()
	fmt.Print("Lerd setup for Windows\n\n")
	rebooting, err := ensureWSLFeatures()
	if err != nil || rebooting {
		return err
	}
	steps := []struct {
		name string
		fn   func() error
	}{
		{"WSL", ensureWSLPackage},
		{setupDistro, ensureDistro},
		{"Linux user", ensureLinuxUser},
		{"lerd", installLerd},
		{"Windows integration", runWSLSetup},
		{"start", startLerd},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}
	fmt.Println("\nLerd is running. The dashboard is at http://lerd.localhost")
	fmt.Println("Open a new terminal and `lerd` works from Windows too.")
	return nil
}

// ensureWSLFeatures enables the two Windows features WSL 2 runs on. They only
// take effect after a reboot, so setup is registered to resume at next login.
func ensureWSLFeatures() (rebooting bool, err error) {
	var off []string
	for _, f := range []string{"Microsoft-Windows-Subsystem-Linux", "VirtualMachinePlatform"} {
		out, err := powershell(fmt.Sprintf(`(Get-WindowsOptionalFeature -Online -FeatureName %s).State`, f))
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(out) != "Enabled" {
			off = append(off, f)
		}
	}
	if len(off) == 0 {
		fmt.Println("✓ WSL features enabled")
		return false, nil
	}
	fmt.Println("→ enabling", strings.Join(off, " and "))
	if _, err := powershell("Enable-WindowsOptionalFeature -Online -NoRestart -All -FeatureName " + strings.Join(off, ",") + " | Out-Null"); err != nil {
		return false, err
	}
	resume := fmt.Sprintf(`"%s" --setup`, selfPath())
	// A fresh profile has no RunOnce key yet; New-Item -Force would wipe one.
	if _, err := powershell(fmt.Sprintf(`$k = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\RunOnce'
if (-not (Test-Path $k)) { New-Item -Path $k | Out-Null }
Set-ItemProperty -Path $k -Name 'lerd-setup' -Value '%s'`, resume)); err != nil {
		return false, err
	}
	fmt.Println("\nWindows needs a restart to finish enabling WSL. Setup carries on after you log back in.")
	if !confirm("Restart now?") {
		fmt.Println("Restart when you are ready; setup resumes at the next login.")
		return true, nil
	}
	return true, run("shutdown.exe", "/r", "/t", "5")
}

// ensureWSLPackage installs WSL itself from Microsoft's GitHub release. The
// wsl.exe Windows ships is a stub, and its Store-based install is unreliable.
func ensureWSLPackage() error {
	if wslInstalled() {
		fmt.Println("✓ WSL installed")
		return nil
	}
	fmt.Println("→ downloading WSL from github.com/microsoft/WSL")
	rel, err := httpGet("https://api.github.com/repos/microsoft/WSL/releases/latest")
	if err != nil {
		return err
	}
	url, err := winshim.WSLMSIURL(rel, runtime.GOARCH)
	if err != nil {
		return err
	}
	msi, err := httpGet(url)
	if err != nil {
		return err
	}
	path := filepath.Join(os.TempDir(), "lerd-wsl.msi")
	if err := os.WriteFile(path, msi, 0o644); err != nil {
		return err
	}
	defer os.Remove(path)
	fmt.Println("→ installing WSL")
	if err := run("msiexec.exe", "/i", path, "/qn", "/norestart"); err != nil {
		return err
	}
	// msiexec returns before Windows Installer finishes registering WSL, and
	// the wsl.exe stub asked in that window offers to install WSL all over again.
	for i := 0; i < 60; i++ {
		if wslInstalled() {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("WSL was installed but wsl.exe still does not report it")
}

func wslInstalled() bool {
	out, err := exec.Command("wsl.exe", "--version").Output()
	return err == nil && strings.Contains(winshim.DecodeWSLOutput(out), "WSL version")
}

func ensureDistro() error {
	out, _ := exec.Command("wsl.exe", "-l", "-q").Output()
	if slices.Contains(winshim.ParseDistroList(out), setupDistro) {
		fmt.Println("✓", setupDistro, "installed")
		return nil
	}
	fmt.Println("→ installing", setupDistro)
	return run("wsl.exe", "--install", "-d", setupDistro, "--no-launch", "--web-download")
}

// ensureLinuxUser gives the distro a sudo user, systemd, and makes that user
// the default. Both settings need the distro restarted, which --terminate does.
func ensureLinuxUser() error {
	user := ""
	if out, err := exec.Command("wsl.exe", "-d", setupDistro, "-u", "root", "--exec", "id", "-nu", "1000").Output(); err == nil {
		user = strings.TrimSpace(string(out))
		fmt.Println("✓ Linux user", user)
	} else {
		def := winshim.LinuxUsername(os.Getenv("USERNAME"))
		user = winshim.LinuxUsername(ask("Linux username", def))
		fmt.Println("→ creating", user, "(choose its password, sudo asks for it)")
		if err := asRoot("useradd -m -s /bin/bash -G sudo " + user); err != nil {
			return err
		}
		if err := run("wsl.exe", "-d", setupDistro, "-u", "root", "--exec", "passwd", user); err != nil {
			return err
		}
	}
	conf := fmt.Sprintf(`f=/etc/wsl.conf; touch $f
grep -q '^systemd=true' $f || printf '\n[boot]\nsystemd=true\n' >> $f
grep -q '^default=' $f || printf '\n[user]\ndefault=%s\n' >> $f`, user)
	if err := asRoot(conf); err != nil {
		return err
	}
	return run("wsl.exe", "--terminate", setupDistro)
}

// installLerd runs lerd's own installer inside the distro, so Windows gets
// exactly what every Linux machine gets. LERD_SETUP_LOCAL names a Windows
// folder holding install.sh, lerd and lerd.exe, for testing a build before it
// is released; the distro is new, so the build has to come from outside it.
func installLerd() error {
	script := "curl -fsSL https://lerd.sh/install.sh | bash"
	if dir := os.Getenv("LERD_SETUP_LOCAL"); dir != "" {
		script = fmt.Sprintf(`cd "$(wslpath '%s')" && bash install.sh --local ./lerd`, dir)
	}
	fmt.Println("→ installing lerd inside", setupDistro)
	return run("wsl.exe", "-d", setupDistro, "--cd", "~", "--exec", "bash", "-lc", script)
}

func runWSLSetup() error {
	return run("wsl.exe", "-d", setupDistro, "--cd", "~", "--exec", "bash", "-lc", "~/.local/bin/lerd wsl:setup")
}

// startLerd restarts WSL so mirrored networking and the resolver handover take
// effect, boots the distro again, and waits for the dashboard to answer.
func startLerd() error {
	fmt.Println("→ restarting WSL")
	if err := run("wsl.exe", "--shutdown"); err != nil {
		return err
	}
	if err := run("wsl.exe", "-d", setupDistro, "--exec", "/bin/true"); err != nil {
		return err
	}
	for i := 0; i < 90; i++ {
		if resp, err := http.Get("http://127.0.0.1:7073/"); err == nil {
			resp.Body.Close()
			fmt.Println("✓ lerd is up")
			return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", "http://lerd.localhost").Run()
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("the dashboard did not answer on 127.0.0.1:7073 within 3 minutes")
}

// asRoot runs script as root. --exec, not --, because `wsl.exe --` hands the
// command to the user's shell first, which expands $vars and eats newlines.
func asRoot(script string) error {
	return run("wsl.exe", "-d", setupDistro, "-u", "root", "--exec", "sh", "-c", script)
}

// isAdmin reports whether this process is elevated: `net session` is refused
// to anyone else.
func isAdmin() bool {
	return exec.Command("net.exe", "session").Run() == nil
}

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		fail(err)
	}
	return p
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func powershell(script string) (string, error) {
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func httpGet(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func ask(question, def string) string {
	fmt.Printf("%s [%s]: ", question, def)
	line, _ := stdin.ReadString('\n')
	if line = strings.TrimSpace(line); line != "" {
		return line
	}
	return def
}

func confirm(question string) bool {
	fmt.Printf("%s [Y/n] ", question)
	line, _ := stdin.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "" || line == "y" || line == "yes"
}

// pause keeps a double-clicked console open long enough to read the outcome.
func pause() {
	fmt.Print("\nPress Enter to close.")
	stdin.ReadString('\n') //nolint:errcheck
}
