//go:build windows

package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

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
		return "", false, fmt.Errorf("no Podman machine backend is ready on this PC.\n\n"+
			"Your Windows edition%s supports Hyper-V, which runs lerd natively. Enable it from an elevated PowerShell, reboot, then run lerd install again:\n\n    %s\n\n"+
			"Or use WSL2 instead: install it, reboot, then run lerd install again:\n\n    %s",
			h.editionLabel(), enableHyperVCmd, installWSLCmd)
	}
	return "", false, fmt.Errorf("no Podman machine backend is ready on this PC.\n\n"+
		"Your Windows edition%s does not include Hyper-V, so lerd runs its containers on WSL2. Install it from an elevated PowerShell, reboot, then run lerd install again:\n\n    %s",
		h.editionLabel(), installWSLCmd)
}

// explicitProviderReady checks a provider the user or a previous install chose.
func explicitProviderReady(source, p string, h hostBackends) error {
	switch {
	case p == machineProviderWSL && !h.wslInstalled:
		return fmt.Errorf("%s is %q, but WSL is not installed. Install it from an elevated PowerShell, reboot, then run lerd install again:\n\n    %s", source, p, installWSLCmd)
	case p == machineProviderWSL && !h.vmPlatform:
		return vmPlatformError()
	case p == machineProviderHyperV && !h.hyperVEdition():
		return fmt.Errorf("%s is %q, but this Windows edition%s does not include Hyper-V. Choose %q instead, then install WSL from an elevated PowerShell, reboot and run lerd install again:\n\n    %s",
			source, p, h.editionLabel(), machineProviderWSL, installWSLCmd)
	case p == machineProviderHyperV && !h.hyperVEnabled:
		return fmt.Errorf("%s is %q, but Hyper-V is not enabled. Enable it from an elevated PowerShell, reboot, then run lerd install again:\n\n    %s", source, p, enableHyperVCmd)
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
    - needs Hyper-V enabled from an elevated PowerShell and a reboot first
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

// hyperVSwitchError ends the install when the user picks Hyper-V on a host
// that does not have it enabled yet.
func hyperVSwitchError() error {
	return errors.New("enable Hyper-V from an elevated PowerShell, reboot, then run lerd install again:\n\n    " + enableHyperVCmd)
}

// machineMemoryFor returns the memory to request for a provider. WSL machines
// take theirs from .wslconfig and podman refuses to set it, so they get none.
func machineMemoryFor(provider string, targetMiB int64) int64 {
	if provider == machineProviderWSL {
		return 0
	}
	return targetMiB
}

// podmanMissingError says how to get the Podman CLI lerd drives.
func podmanMissingError() error {
	return errors.New("lerd drives the Podman CLI, which is not on your PATH. Install it, open a new terminal, then run lerd install again:\n\n    " + installPodman)
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
