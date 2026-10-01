//go:build windows

package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

const (
	enableHyperV = "Enable-WindowsOptionalFeature"
	installWSL   = "wsl --install"
)

var (
	proBare      = hostBackends{edition: "Professional"}
	proHyperV    = hostBackends{edition: "Professional", hyperVEnabled: true}
	proWSL       = hostBackends{edition: "Professional", wslInstalled: true, vmPlatform: true}
	homeBare     = hostBackends{edition: "Core"}
	homeWSL      = hostBackends{edition: "Core", wslInstalled: true, vmPlatform: true}
	homeWSLNoVM  = hostBackends{edition: "Core", wslInstalled: true}
	proBothReady = hostBackends{edition: "Professional", hyperVEnabled: true, wslInstalled: true, vmPlatform: true}
)

func TestPlanMachineProviderPicksAReadyBackend(t *testing.T) {
	cases := []struct {
		name, env, saved string
		host             hostBackends
		want             string
	}{
		{"hyper-v enabled is the native default", "", "", proHyperV, "hyperv"},
		{"hyper-v wins when both are ready", "", "", proBothReady, "hyperv"},
		{"home with wsl falls back to wsl", "", "", homeWSL, "wsl"},
		{"pro without hyper-v uses the wsl it has", "", "", proWSL, "wsl"},
		{"saved choice beats detection", "", "wsl", proBothReady, "wsl"},
		{"explicit env beats the saved choice", "wsl", "hyperv", proBothReady, "wsl"},
		{"values are matched case-insensitively", " HyperV ", "", proHyperV, "hyperv"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, err := planMachineProvider(c.env, c.saved, c.host)
			if err != nil || got != c.want {
				t.Errorf("got %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

// A host that already has WSL but could run Hyper-V is offered the choice,
// once, and only while nobody has decided yet.
func TestPlanMachineProviderOffersHyperVToAWSLHost(t *testing.T) {
	cases := []struct {
		name, env, saved string
		host             hostBackends
		provider         string
		offer            bool
	}{
		{"pro with only wsl ready", "", "", proWSL, "wsl", true},
		{"home has no hyper-v to offer", "", "", homeWSL, "wsl", false},
		{"hyper-v already enabled", "", "", proBothReady, "hyperv", false},
		{"wsl already saved", "", "wsl", proWSL, "wsl", false},
		{"wsl chosen in the environment", "wsl", "", proWSL, "wsl", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, offer, err := planMachineProvider(c.env, c.saved, c.host)
			if err != nil || p != c.provider || offer != c.offer {
				t.Errorf("got %q, offer=%v, %v; want %q, offer=%v", p, offer, err, c.provider, c.offer)
			}
		})
	}
}

func TestProviderComparisonShowsBothSides(t *testing.T) {
	text := providerComparison()
	for _, want := range []string{"Hyper-V (recommended)", "WSL2", "wsl --shutdown", ".wslconfig", "reboot"} {
		if !strings.Contains(text, want) {
			t.Errorf("comparison missing %q:\n%s", want, text)
		}
	}
}

// A scripted install has nobody to answer, so it must carry on with WSL rather
// than stop on the recommended default.
func TestAskHyperVOverWSLKeepsWSLWithoutATerminal(t *testing.T) {
	old := stdinIsTerminal
	stdinIsTerminal = func() bool { return false }
	t.Cleanup(func() { stdinIsTerminal = old })
	if askHyperVOverWSL() {
		t.Error("no terminal: chose Hyper-V, want WSL")
	}
}

func TestHyperVSwitchErrorSaysHowAndToRerun(t *testing.T) {
	msg := hyperVSwitchError().Error()
	if !strings.Contains(msg, enableHyperV) || !strings.Contains(msg, "lerd install") {
		t.Errorf("switch guidance: %q", msg)
	}
}

// WSL machines take their memory from .wslconfig; podman refuses to set it.
func TestMachineMemoryForSkipsWSL(t *testing.T) {
	if got := machineMemoryFor("hyperv", 6144); got != 6144 {
		t.Errorf("hyperv: got %d", got)
	}
	if got := machineMemoryFor("wsl", 6144); got != 0 {
		t.Errorf("wsl: got %d, want 0", got)
	}
}

func TestPlanMachineProviderGuidesWhenNothingIsReady(t *testing.T) {
	_, _, err := planMachineProvider("", "", proBare)
	if err == nil {
		t.Fatal("pro with no backend must stop the install")
	}
	for _, want := range []string{enableHyperV, installWSL, "lerd install"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("pro guidance missing %q:\n%v", want, err)
		}
	}

	_, _, err = planMachineProvider("", "", homeBare)
	if err == nil {
		t.Fatal("home with no backend must stop the install")
	}
	if msg := err.Error(); !strings.Contains(msg, installWSL) || strings.Contains(msg, enableHyperV) || !strings.Contains(msg, "lerd install") {
		t.Errorf("home guidance should offer only WSL and the rerun:\n%v", msg)
	}
}

// WSL installed without the Host Compute Service cannot create the machine
// (HCS_E_SERVICE_NOT_AVAILABLE), so it is not ready, and installing WSL again
// would not help: the user is told how to repair the VM platform.
func TestPlanMachineProviderRepairsAMissingVMPlatform(t *testing.T) {
	for name, run := range map[string]func() error{
		"auto":     func() error { _, _, err := planMachineProvider("", "", homeWSLNoVM); return err },
		"explicit": func() error { _, _, err := planMachineProvider("wsl", "", homeWSLNoVM); return err },
	} {
		err := run()
		if err == nil {
			t.Fatalf("%s: wsl without the VM platform must stop the install", name)
		}
		msg := err.Error()
		for _, want := range []string{"VirtualMachinePlatform", "hypervisorlaunchtype auto", "lerd install"} {
			if !strings.Contains(msg, want) {
				t.Errorf("%s: guidance missing %q:\n%s", name, want, msg)
			}
		}
		if strings.Contains(msg, installWSL) {
			t.Errorf("%s: WSL is installed, guidance should not ask to install it:\n%s", name, msg)
		}
	}
}

func TestPlanMachineProviderChecksAnExplicitChoice(t *testing.T) {
	if _, _, err := planMachineProvider("hyperv", "", homeWSL); err == nil || !strings.Contains(err.Error(), installWSL) {
		t.Errorf("hyperv on home: err = %v, want a pointer to WSL", err)
	}
	if _, _, err := planMachineProvider("hyperv", "", proWSL); err == nil || !strings.Contains(err.Error(), enableHyperV) {
		t.Errorf("hyperv off on pro: err = %v, want the enable command", err)
	}
	if _, _, err := planMachineProvider("", "wsl", proHyperV); err == nil || !strings.Contains(err.Error(), installWSL) {
		t.Errorf("saved wsl without wsl: err = %v, want the install command", err)
	}
}

func TestPlanMachineProviderRefusesUnknownValues(t *testing.T) {
	if _, _, err := planMachineProvider("qemu", "", proBothReady); err == nil || !strings.Contains(err.Error(), "qemu") {
		t.Errorf("unknown env value: err = %v, want one naming qemu", err)
	}
	if _, _, err := planMachineProvider("", "applehv", proBothReady); err == nil || !strings.Contains(err.Error(), "applehv") {
		t.Errorf("unknown saved value: err = %v, want one naming applehv", err)
	}
}

func TestHyperVEdition(t *testing.T) {
	for edition, want := range map[string]bool{
		"Core": false, "CoreN": false, "CoreSingleLanguage": false, "CoreCountrySpecific": false,
		"Professional": true, "Enterprise": true, "Education": true, "ProfessionalWorkstation": true,
		"": true, // unreadable edition: let the Hyper-V check decide
	} {
		if got := (hostBackends{edition: edition}).hyperVEdition(); got != want {
			t.Errorf("hyperVEdition(%q) = %v, want %v", edition, got, want)
		}
	}
}

func TestMachineProviderReadsTheSavedChoice(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(machineProviderEnv, "")
	stubHost(t, proBothReady)

	if got, _, _ := machineProvider(); got != "hyperv" {
		t.Fatalf("nothing saved on a hyper-v host: got %q", got)
	}
	if err := rememberMachineProvider("wsl"); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := machineProvider(); got != "wsl" {
		t.Errorf("after saving wsl: got %q", got)
	}
	cfg, err := config.LoadGlobal()
	if err != nil || cfg.Machine.Provider != "wsl" {
		t.Errorf("saved provider = %q, %v", cfg.Machine.Provider, err)
	}
}

func TestPodmanMissingErrorSaysHowToInstallIt(t *testing.T) {
	msg := podmanMissingError().Error()
	if !strings.Contains(msg, "winget install RedHat.Podman") || !strings.Contains(msg, "lerd install") {
		t.Errorf("podman guidance: %q", msg)
	}
}

func TestMachineInitHintNamesWhatEachProviderNeeds(t *testing.T) {
	if h := machineInitHint("hyperv"); !strings.Contains(h, "elevated") {
		t.Errorf("hyperv hint %q should mention an elevated shell", h)
	}
	if h := machineInitHint("wsl"); !strings.Contains(h, installWSL) {
		t.Errorf("wsl hint %q should name wsl --install", h)
	}
}

func stubHost(t *testing.T, h hostBackends) {
	t.Helper()
	old := detectHost
	detectHost = func() hostBackends { return h }
	t.Cleanup(func() { detectHost = old })
}
