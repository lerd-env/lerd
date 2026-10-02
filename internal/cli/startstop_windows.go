//go:build windows

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
)

// migrateExecWorkerPlists is a no-op: Windows never had plist-based workers.
func migrateExecWorkerPlists() {}

// traySessionAvailable is always true: a Windows login is the session.
func traySessionAvailable() bool { return true }

// memoryStatusEx mirrors the Win32 MEMORYSTATUSEX struct.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var procGlobalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// hostMemoryGiB returns installed RAM in GiB, or 0 when Windows will not say,
// so the caller falls back to the safe default.
func hostMemoryGiB() int {
	st := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&st))); r == 0 {
		return 0
	}
	return int(st.TotalPhys / (1024 * 1024 * 1024))
}

// machineInitArgs builds `podman machine init`. A rootful machine is needed to
// publish ports 80 and 443. Host drives are not mounted by flag here: both the
// Hyper-V and WSL providers already expose C:\ at /mnt/c.
func machineInitArgs(name string, targetMemoryMiB int64) []string {
	args := []string{"machine", "init", "--rootful"}
	if targetMemoryMiB > 0 {
		args = append(args, "--memory", strconv.FormatInt(targetMemoryMiB, 10))
	}
	if name != "" {
		args = append(args, name)
	}
	return args
}

// ensurePodmanMachineRunning brings the machine up on the provider the host
// supports and, on WSL, makes sure containers can actually start in it. Both
// lerd install and lerd start pass through here, so a machine broken by a WSL
// update is repaired on the next start too. A host with nothing ready stops
// here with the steps to enable a backend.
func ensurePodmanMachineRunning() error {
	if _, err := exec.LookPath("podman"); err != nil {
		return podmanMissingError()
	}
	provider, offerHyperV, err := machineProvider()
	if err != nil {
		return err
	}
	if err := os.Setenv(machineProviderEnv, provider); err != nil {
		return err
	}
	if err := bringMachineUp(provider, offerHyperV); err != nil {
		return err
	}
	if provider == machineProviderWSL {
		return ensureWSLContainersRun()
	}
	// Hyper-V shares the drives through Podman's 9p server, which breaks file
	// replaces, appends and locks; lerd serves them itself instead.
	if err := takeOverP9Shares(selectedMachineName()); err != nil {
		feedback.Warn("%v", err)
	}
	return nil
}

// bringMachineUp creates the machine on first run, makes it rootful and big
// enough, starts it, then waits for the API.
func bringMachineUp(provider string, offerHyperV bool) error {
	name, running, rootful := selectedMachineState()

	cfg, _ := config.LoadGlobal()
	execMode := cfg != nil && cfg.WorkerExecMode() != config.WorkerExecModeContainer
	targetMiB := machineMemoryFor(provider, recommendedVMMemoryMiB(hostMemoryGiB(), execMode))

	if name == "" {
		if offerHyperV && askHyperVOverWSL() {
			return hyperVSwitchError()
		}
		feedback.Line(fmt.Sprintf("Initialising Podman Machine on %s (first run, this may take a minute)…", provider))
		cmd := podman.Cmd(machineInitArgs("", targetMiB)...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("podman machine init (%s): %w", machineInitHint(provider), err)
		}
		if err := rememberMachineProvider(provider); err != nil {
			feedback.Warn("could not save the machine provider (%v); set %s=%s if lerd loses the machine", err, machineProviderEnv, provider)
		}
	} else {
		needsMemory := targetMiB > 0 && machineMemoryMiB(name) > 0 && machineMemoryMiB(name) < targetMiB
		if rootful && !needsMemory && running {
			return nil
		}
		if running {
			feedback.Line("Stopping Podman Machine to apply its settings…")
			_ = runMachineStreaming(machineStopTimeout, "machine", "stop", name)
		}
		if !rootful {
			runMachineSet("--rootful", name)
		}
		if needsMemory {
			runMachineSet("--memory", strconv.FormatInt(targetMiB, 10), name)
		}
	}

	feedback.Line("Starting Podman Machine…")
	if err := startPodmanMachineWithRetry(); err != nil {
		return err
	}
	waitForMachineAPI()
	return nil
}

// selectedMachineState returns the machine lerd uses (default-marked, else
// first) with whether it is running and rootful; the name is "" when none exists.
func selectedMachineState() (name string, running, rootful bool) {
	name = selectedMachineName()
	if name == "" {
		return "", false, false
	}
	if out, err := machineQuery("machine", "list", "--format", "{{.Name}}\t{{.Running}}"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && strings.TrimSuffix(f[0], "*") == name {
				running = f[1] == "true"
			}
		}
	}
	if out, err := machineQuery("machine", "inspect", "--format", "{{.Rootful}}", name); err == nil {
		rootful = strings.TrimSpace(string(out)) == "true"
	}
	return name, running, rootful
}

func machineMemoryMiB(name string) int64 {
	out, err := machineQuery("machine", "inspect", "--format", "{{.Resources.Memory}}", name)
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return n
}

func runMachineSet(args ...string) {
	cmd := podman.Cmd(append([]string{"machine", "set"}, args...)...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		feedback.Warn("podman machine set %s: %v", strings.Join(args, " "), err)
	}
}

// startPodmanMachineWithRetry runs `podman machine start`, retrying once, and
// treats a machine that already answers `podman ps` as started.
func startPodmanMachineWithRetry() error {
	run := func() error { return runMachineStreaming(machineStartTimeout, "machine", "start") }
	err := run()
	if err == nil || machineAlreadyUsable() {
		return nil
	}
	feedback.Warn("podman machine start: %v", err)
	feedback.Line("Retrying Podman Machine start once…")
	time.Sleep(3 * time.Second)
	if err = run(); err != nil {
		feedback.Note("Try: podman machine stop; podman machine start. If it keeps failing, run `lerd machine reset` to recreate the VM, then `lerd install` again.")
		return fmt.Errorf("podman machine start: %w", err)
	}
	return nil
}

// machineAlreadyUsable reports whether container operations work right now;
// `podman ps` exercises the whole stack rather than a status field.
func machineAlreadyUsable() bool {
	return podman.Cmd("ps", "-q").Run() == nil
}

// waitForMachineAPI polls until `podman ps` succeeds, since `machine start`
// returns before the API socket handles container operations.
func waitForMachineAPI() {
	fmt.Printf(" %s %s", feedback.Dim("→"), feedback.Dim("Waiting for Podman Machine to be ready…"))
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		if machineAlreadyUsable() {
			time.Sleep(3 * time.Second)
			fmt.Println(" " + feedback.Green("ready"))
			return
		}
		time.Sleep(500 * time.Millisecond)
		fmt.Print(feedback.Dim("."))
	}
	fmt.Println(" " + feedback.Amber("timed out (proceeding anyway)"))
}
