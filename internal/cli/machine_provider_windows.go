//go:build windows

package cli

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/term"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

// machineProviderEnv is how Podman 5+ selects its VM backend. Every podman
// machine call reads it, so it is exported for the whole process at startup.
const (
	machineProviderEnv    = "CONTAINERS_MACHINE_PROVIDER"
	machineProviderHyperV = "hyperv"
	machineProviderWSL    = "wsl"

	enableHyperVCmd = "Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V -All"
	installWSLCmd   = "wsl --install --no-distribution"
	installPodman   = "winget install RedHat.Podman"
)

func init() {
	if p, _, err := machineProvider(); err == nil {
		os.Setenv(machineProviderEnv, p) //nolint:errcheck
	}
}

// hostBackends is what this PC offers for running the Podman machine.
type hostBackends struct {
	edition       string // the registry EditionID, "Core" on Home
	hyperVEnabled bool
	wslInstalled  bool
	vmPlatform    bool // the Host Compute Service WSL2 creates its VM through
}

// wslReady reports whether WSL can create a VM, not just that it is installed.
func (h hostBackends) wslReady() bool { return h.wslInstalled && h.vmPlatform }

// hyperVEdition reports whether this Windows edition ships Hyper-V. Home is
// "Core" plus a suffix; an unreadable edition is not held against the host.
func (h hostBackends) hyperVEdition() bool {
	return !strings.HasPrefix(h.edition, "Core")
}

func (h hostBackends) editionLabel() string {
	switch {
	case h.edition == "":
		return ""
	case !h.hyperVEdition():
		return " (Home)"
	}
	return " (" + h.edition + ")"
}

// machineProvider returns the backend lerd's Podman machine uses, whether the
// user should be offered Hyper-V instead, or the steps to get one ready.
func machineProvider() (provider string, offerHyperV bool, err error) {
	saved := ""
	if cfg, err := config.LoadGlobal(); err == nil {
		saved = cfg.Machine.Provider
	}
	return planMachineProvider(os.Getenv(machineProviderEnv), saved, detectHost())
}

// planMachineProvider honours an explicit environment value, then the provider
// saved when the machine was created, then picks Hyper-V (native) or WSL from
// what the host has ready. A WSL host that could run Hyper-V gets WSL with
// offerHyperV set, so the user decides. When nothing is ready, the error tells
// the user how to enable a backend and to run lerd install again.
func planMachineProvider(env, saved string, h hostBackends) (provider string, offerHyperV bool, err error) {
	for _, v := range []struct{ source, value string }{{machineProviderEnv, env}, {"machine.provider", saved}} {
		p := strings.ToLower(strings.TrimSpace(v.value))
		switch p {
		case "":
			continue
		case machineProviderHyperV, machineProviderWSL:
			return p, false, explicitProviderReady(v.source, p, h)
		default:
			return "", false, fmt.Errorf("%s is %q; lerd supports %q or %q", v.source, v.value, machineProviderHyperV, machineProviderWSL)
		}
	}
	switch {
	case h.hyperVEnabled:
		return machineProviderHyperV, false, nil
	case h.wslReady():
		return machineProviderWSL, h.hyperVEdition(), nil
	case h.wslInstalled:
		return "", false, vmPlatformError()
	case h.hyperVEdition():
		return "", false, &backendSetup{provider: machineProviderHyperV, orWSL: true, guidance: fmt.Sprintf("no Podman machine backend is ready on this PC.\n\n"+
			"Your Windows edition%s supports Hyper-V, which runs lerd natively. Enable it from an elevated PowerShell, reboot, then run lerd install again:\n\n    %s\n\n"+
			"Or use WSL2 instead: install it, reboot, then run lerd install again:\n\n    %s",
			h.editionLabel(), enableHyperVCmd, installWSLCmd)}
	}
	return "", false, &backendSetup{provider: machineProviderWSL, guidance: fmt.Sprintf("no Podman machine backend is ready on this PC.\n\n"+
		"Your Windows edition%s does not include Hyper-V, so lerd runs its containers on WSL2. Install it from an elevated PowerShell, reboot, then run lerd install again:\n\n    %s",
		h.editionLabel(), installWSLCmd)}
}

// explicitProviderReady checks a provider the user or a previous install chose.
func explicitProviderReady(source, p string, h hostBackends) error {
	switch {
	case p == machineProviderWSL && !h.wslInstalled:
		return &backendSetup{provider: p, guidance: fmt.Sprintf("%s is %q, but WSL is not installed. Install it from an elevated PowerShell, reboot, then run lerd install again:\n\n    %s", source, p, installWSLCmd)}
	case p == machineProviderWSL && !h.vmPlatform:
		return vmPlatformError()
	case p == machineProviderHyperV && !h.hyperVEdition():
		return fmt.Errorf("%s is %q, but this Windows edition%s does not include Hyper-V. Choose %q instead, then install WSL from an elevated PowerShell, reboot and run lerd install again:\n\n    %s",
			source, p, h.editionLabel(), machineProviderWSL, installWSLCmd)
	case p == machineProviderHyperV && !h.hyperVEnabled:
		return &backendSetup{provider: p, guidance: fmt.Sprintf("%s is %q, but Hyper-V is not enabled. Enable it from an elevated PowerShell, reboot, then run lerd install again:\n\n    %s", source, p, enableHyperVCmd)}
	}
	return nil
}

// vmPlatformError covers WSL that is installed but cannot start a VM because
// the Host Compute Service is missing, which reinstalling WSL does not fix.
func vmPlatformError() error {
	return errors.New("WSL is installed, but Windows cannot start virtual machines: the Host Compute Service (vmcompute) is missing. " +
		"Repair the Virtual Machine Platform from an elevated PowerShell:\n\n" +
		"    dism.exe /online /enable-feature /featurename:VirtualMachinePlatform /all /norestart\n" +
		"    bcdedit /set hypervisorlaunchtype auto\n\n" +
		"Then reboot, make sure virtualization is enabled in the BIOS/UEFI, and run lerd install again.")
}

// providerComparison is shown before asking a WSL host whether to switch to
// Hyper-V, so the user can weigh both.
func providerComparison() string {
	return `This PC already has WSL2, and its Windows edition also supports Hyper-V. Lerd can run its Podman machine on either:

  Hyper-V (recommended)
    + its own VM, separate from your WSL distros, so 'wsl --shutdown' or a WSL update leaves your sites running
    + lerd sizes the VM's memory for this PC
    - lerd enables it for you, then Windows needs a reboot first
    - creating the machine needs an elevated shell

  WSL2
    + already installed, works right now with no reboot
    + no elevated shell needed to create the machine
    - shares the WSL2 VM with your other distros, so 'wsl --shutdown' stops your sites too
    - memory follows your .wslconfig, not lerd's sizing
    - less tested with lerd on Windows`
}

// askHyperVOverWSL shows both options and asks whether to switch to Hyper-V.
// With no terminal to ask on, it keeps WSL so a scripted install still works.
// promptSource is not used: on Windows it takes NUL for a terminal.
func askHyperVOverWSL() bool {
	fmt.Println(providerComparison())
	if !stdinIsTerminal() {
		feedback.Note("no terminal to ask on, continuing with WSL2. To switch to Hyper-V later, enable it, run 'podman machine rm' and run lerd install again")
		return false
	}
	return readConfirmAnswer(os.Stdin, "Enable Hyper-V and run lerd natively (recommended)?", true)
}

// stdinIsTerminal is a var so tests can stand in for a console.
var stdinIsTerminal = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// hyperVSwitch is the setup run when the user picks Hyper-V on a host that
// does not have it enabled yet.
func hyperVSwitch() *backendSetup {
	return &backendSetup{provider: machineProviderHyperV, guidance: "enable Hyper-V from an elevated PowerShell, reboot, then run lerd install again:\n\n    " + enableHyperVCmd}
}

// backendSetup is a backend lerd can turn on itself. Its error text is the
// manual route, shown when lerd cannot enable it.
type backendSetup struct {
	provider string
	orWSL    bool // Hyper-V is recommended, the user may take WSL2 instead
	guidance string
}

func (b *backendSetup) Error() string { return b.guidance }

// enableScripts run elevated. -NoRestart keeps DISM from rebooting or asking
// to; 3010 is Windows' "succeeded, reboot required".
var enableScripts = map[string]string{
	machineProviderHyperV: enableHyperVCmd + " -NoRestart -ErrorAction Stop | Out-Null",
	machineProviderWSL:    installWSLCmd + "; if ($LASTEXITCODE -ne 3010) { exit $LASTEXITCODE }",
}

// enableBackend turns the backend on through a UAC prompt, saves it as the
// provider so the next run does not ask again, and ends the run with the
// reboot Windows needs before it can start the VM.
func enableBackend(s *backendSetup) error {
	p := s.provider
	if s.orWSL && !askEnableHyperV() {
		p = machineProviderWSL
	}
	name := providerLabel(p)
	feedback.Line(fmt.Sprintf("Enabling %s, Windows will ask for administrator permission…", name))
	if err := runElevated(enableScripts[p]); err != nil {
		return fmt.Errorf("could not enable %s: %w\n\n%s", name, err, s.guidance)
	}
	if err := rememberMachineProvider(p); err != nil {
		feedback.Warn("could not save the machine provider (%v); set %s=%s after the restart", err, machineProviderEnv, p)
	}
	return fmt.Errorf("%s is enabled. Restart Windows to finish, then run lerd install again", name)
}

func providerLabel(p string) string {
	if p == machineProviderHyperV {
		return "Hyper-V"
	}
	return "WSL2"
}

// askEnableHyperV asks a host with neither backend which one to enable. With
// no terminal it takes the recommended Hyper-V.
func askEnableHyperV() bool {
	if !stdinIsTerminal() {
		return true
	}
	return readConfirmAnswer(os.Stdin, "Enable Hyper-V and run lerd natively (recommended)? Answer no to install WSL2 instead", true)
}

// runElevated runs a PowerShell script as administrator and waits for it,
// asking through UAC when lerd is not elevated. A var so tests stay off the host.
var runElevated = func(script string) error {
	enc := encodePowerShell(script)
	var cmd *exec.Cmd
	if windows.GetCurrentProcessToken().IsElevated() {
		cmd = exec.Command("powershell.exe", "-NoProfile", "-EncodedCommand", enc)
	} else {
		cmd = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
			"$p = Start-Process powershell.exe -Verb RunAs -Wait -PassThru -ArgumentList '-NoProfile','-EncodedCommand','"+enc+"'; exit $p.ExitCode")
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// encodePowerShell is the base64 UTF-16LE form -EncodedCommand takes, so the
// script needs no quoting through Start-Process.
func encodePowerShell(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// machineMemoryFor returns the memory to request for a provider. WSL machines
// take theirs from .wslconfig and podman refuses to set it, so they get none.
func machineMemoryFor(provider string, targetMiB int64) int64 {
	if provider == machineProviderWSL {
		return 0
	}
	return targetMiB
}

// podmanMissingError says why lerd could not install the Podman CLI it drives,
// and how to install it by hand.
func podmanMissingError(cause error) error {
	return fmt.Errorf("lerd drives the Podman CLI and could not install it: %w\n\nInstall it yourself, open a new terminal, then run lerd install again:\n\n    %s", cause, installPodman)
}

// rememberMachineProvider saves the provider a new machine was created with,
// so later processes keep finding it after the host changes.
func rememberMachineProvider(p string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	if cfg.Machine.Provider == p {
		return nil
	}
	cfg.Machine.Provider = p
	return config.SaveGlobal(cfg)
}

// machineInitHint says what a failed `podman machine init` most likely lacked.
func machineInitHint(provider string) string {
	if provider == machineProviderHyperV {
		return "Hyper-V machines need an elevated shell"
	}
	return "the WSL provider needs WSL, run '" + installWSLCmd + "' from an elevated PowerShell and reboot"
}

// detectHost reads the edition and which backends are installed, with no
// elevation needed. A var so tests can pin the host.
var detectHost = func() hostBackends {
	h := hostBackends{
		hyperVEnabled: windowsServiceExists("vmms"),
		wslInstalled:  windowsServiceExists("WSLService") || windowsServiceExists("LxssManager"),
		vmPlatform:    windowsServiceExists("vmcompute"),
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE); err == nil {
		h.edition, _, _ = k.GetStringValue("EditionID")
		k.Close()
	}
	return h
}

// windowsServiceExists reports whether a Windows service exists, which for vmms
// and the WSL services means the matching feature is on.
func windowsServiceExists(name string) bool {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(scm) //nolint:errcheck
	n, _ := windows.UTF16PtrFromString(name)
	svc, err := windows.OpenService(scm, n, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false
	}
	windows.CloseServiceHandle(svc) //nolint:errcheck
	return true
}
