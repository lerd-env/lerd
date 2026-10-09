package cli

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// reportProbeTimeout bounds every external call the Windows report makes, so a
// missing podman or a wedged VM costs one line in the report, not a hang.
const reportProbeTimeout = 20 * time.Second

// windowsHostLogs are the host logs worth reading that are not units: the 9p
// server and its guard, and the msiexec log of a Podman install lerd ran.
var windowsHostLogs = map[string]bool{"p9-serve": true, "podman-install": true}

// writeHostDetails adds the Windows release to the bug report header.
func writeHostDetails(w io.Writer) {
	fmt.Fprintf(w, "Windows:    %s\n", readWindowsVersion())
}

func readWindowsVersion() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return windowsVersionLabel("", "", "", "", 0)
	}
	defer k.Close()
	product, _, _ := k.GetStringValue("ProductName")
	edition, _, _ := k.GetStringValue("EditionID")
	display, _, _ := k.GetStringValue("DisplayVersion")
	build, _, _ := k.GetStringValue("CurrentBuildNumber")
	ubr, _, _ := k.GetIntegerValue("UBR")
	return windowsVersionLabel(product, edition, display, build, ubr)
}

// windowsVersionLabel names the release. Windows 11 kept "Windows 10" in
// ProductName, so a build from 22000 on is relabelled.
func windowsVersionLabel(product, edition, display, build string, ubr uint64) string {
	if product == "" && build == "" {
		return "(unreadable)"
	}
	if n, err := strconv.Atoi(build); err == nil && n >= 22000 {
		product = strings.Replace(product, "Windows 10", "Windows 11", 1)
	}
	label := strings.TrimSpace(product + " " + display)
	return fmt.Sprintf("%s (build %s.%d, edition %s)", label, build, ubr, edition)
}

// writePlatformSections adds what decides whether lerd can run on this PC: the
// host, the virtualization backends, WSL, the Podman machine and lerd's own
// install. Each probe stands alone, so a half-finished install still reports.
func writePlatformSections(w io.Writer) {
	section(w, "Windows host")
	dumpWindowsHost(w)

	section(w, "Virtualization")
	dumpVirtualization(w)

	section(w, "Podman machine")
	dumpPodmanMachine(w)

	section(w, "lerd install")
	dumpLerdInstall(w)
}

func dumpWindowsHost(w io.Writer) {
	token := windows.GetCurrentProcessToken()
	fmt.Fprintf(w, "Elevated:          %s\n", yesNo(token.IsElevated()))
	if sid, err := windows.CreateWellKnownSid(windows.WinBuiltinHyperVAdminsSid); err == nil {
		member, err := windows.Token(0).IsMember(sid)
		if err != nil {
			fmt.Fprintf(w, "Hyper-V Admins:    (unknown: %v)\n", err)
		} else {
			fmt.Fprintf(w, "Hyper-V Admins:    %s\n", yesNo(member))
		}
	}
	facts, err := hostFacts()
	if err != nil {
		fmt.Fprintf(w, "Hardware:          (unavailable: %v)\n", err)
		return
	}
	fmt.Fprintf(w, "CPU:               %s (%s logical)\n", facts["cpu"], facts["logical"])
	if b, err := strconv.ParseUint(facts["memory"], 10, 64); err == nil {
		fmt.Fprintf(w, "Memory:            %s\n", formatGiB(b))
	}
	fmt.Fprintf(w, "Hypervisor:        %s\n", presentOrNot(facts["hypervisor"] == "True"))
	fmt.Fprintf(w, "PowerShell:        %s\n", facts["psversion"])
}

// hostFacts reads the hardware lines in one PowerShell call, as key=value.
func hostFacts() (map[string]string, error) {
	script := `$cs = Get-CimInstance Win32_ComputerSystem; $p = Get-CimInstance Win32_Processor | Select-Object -First 1
"cpu=$($p.Name.Trim())"; "logical=$($cs.NumberOfLogicalProcessors)"; "memory=$($cs.TotalPhysicalMemory)"
"hypervisor=$($cs.HypervisorPresent)"; "psversion=$($PSVersionTable.PSVersion)"`
	out, _, err := runProbe(exec.CommandContext, "powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShell(script))
	if err != nil {
		return nil, err
	}
	facts := map[string]string{}
	for _, line := range strings.Split(decodeConsoleText(out), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			facts[k] = v
		}
	}
	return facts, nil
}

func dumpVirtualization(w io.Writer) {
	h := detectHost()
	fmt.Fprintf(w, "Edition supports Hyper-V:  %s\n", yesNo(h.hyperVEdition()))
	fmt.Fprintf(w, "Hyper-V (vmms):            %s\n", enabledOrNot(h.hyperVEnabled))
	fmt.Fprintf(w, "WSL:                       %s\n", installedOrNot(h.wslInstalled))
	fmt.Fprintf(w, "VM Platform (vmcompute):   %s\n", presentOrNot(h.vmPlatform))

	saved := ""
	if cfg, err := config.LoadGlobal(); err == nil {
		saved = cfg.Machine.Provider
	}
	fmt.Fprintf(w, "machine.provider (config): %s\n", orNone(saved))
	if p, _, err := machineProvider(); err != nil {
		fmt.Fprintf(w, "Provider lerd would use:   (none ready)\n%s\n", err)
	} else {
		fmt.Fprintf(w, "Provider lerd would use:   %s\n", p)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "── wsl --version")
	if !h.wslInstalled {
		fmt.Fprintln(w, "(WSL not installed)")
		return
	}
	out, errOut, err := runProbe(exec.CommandContext, "wsl.exe", "--version")
	if text := strings.TrimSpace(decodeConsoleText(append(out, errOut...))); text != "" {
		fmt.Fprintln(w, text)
	} else if err != nil {
		fmt.Fprintf(w, "(failed: %v)\n", err)
	}
}

func dumpPodmanMachine(w io.Writer) {
	out, errOut, err := runProbe(podmanProbe, "version", "--format", "client {{.Client.Version}} ({{.Client.OsArch}}), server {{.Server.Version}} ({{.Server.OsArch}})")
	if text := strings.TrimSpace(string(out)); text != "" {
		fmt.Fprintf(w, "podman:   %s\n", text)
	} else {
		fmt.Fprintf(w, "podman:   (unavailable: %s)\n", probeError(err, errOut))
	}

	out, errOut, err = runProbe(podmanProbe, "machine", "list", "--format", "json")
	if err != nil {
		fmt.Fprintf(w, "machine:  (podman machine list failed: %s)\n", probeError(err, errOut))
	} else {
		fmt.Fprintf(w, "machine:  %s\n", summarizeMachineList(out))
	}
	// podman warns here when it cannot read a Hyper-V machine's state, which is
	// the commonest reason lerd cannot start or stop it.
	if warn := strings.TrimSpace(string(errOut)); warn != "" && err == nil {
		fmt.Fprintf(w, "warning:  %s\n", warn)
		if strings.Contains(warn, "unable to get state") {
			fmt.Fprintln(w, "note:     podman could not read the VM state, so the state and last up above may be stale")
		}
	}

	out, _, err = runProbe(podmanProbe, "system", "connection", "list", "--format", "{{.Name}}\t{{.URI}}\tdefault={{.Default}}")
	if err == nil && len(bytes.TrimSpace(out)) > 0 {
		fmt.Fprintln(w, "\n── podman system connection list")
		fmt.Fprintln(w, strings.TrimSpace(string(out)))
	}
}

type machineEntry struct {
	Name               string
	Default            bool
	Running            bool
	Starting           bool
	LastUp             string
	VMType             string
	CPUs               uint64
	Memory             json.Number
	DiskSize           json.Number
	UserModeNetworking bool
}

// summarizeMachineList renders `podman machine list --format json` as one line
// per machine.
func summarizeMachineList(js []byte) string {
	var list []machineEntry
	dec := json.NewDecoder(bytes.NewReader(js))
	dec.UseNumber()
	if err := dec.Decode(&list); err != nil {
		return fmt.Sprintf("(unparseable machine list: %v)", err)
	}
	if len(list) == 0 {
		return "(no podman machine)"
	}
	var lines []string
	for _, m := range list {
		name := m.Name
		if m.Default {
			name += " (default)"
		}
		state := "stopped"
		switch {
		case m.Starting:
			state = "starting"
		case m.Running:
			state = "running"
		}
		parts := []string{name, "provider " + m.VMType, state, fmt.Sprintf("%d CPUs", m.CPUs)}
		if b, err := strconv.ParseUint(m.Memory.String(), 10, 64); err == nil {
			parts = append(parts, formatGiB(b)+" memory")
		}
		if b, err := strconv.ParseUint(m.DiskSize.String(), 10, 64); err == nil {
			parts = append(parts, formatGiB(b)+" disk")
		}
		umn := "off"
		if m.UserModeNetworking {
			umn = "on"
		}
		parts = append(parts, "user-mode networking "+umn, "last up "+m.LastUp)
		lines = append(lines, strings.Join(parts, ", "))
	}
	return strings.Join(lines, "\n          ")
}

func dumpLerdInstall(w io.Writer) {
	exe, _ := os.Executable()
	fmt.Fprintf(w, "Running binary:    %s\n", exe)
	installed := filepath.Join(config.BinDir(), "lerd.exe")
	fmt.Fprintf(w, "Installed binary:  %s (%s)\n", installed, existsOrMissing(installed))
	tray := filepath.Join(config.BinDir(), "lerd-tray.exe")
	fmt.Fprintf(w, "Tray binary:       %s (%s)\n", tray, existsOrMissing(tray))

	onPath := false
	if k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE); err == nil {
		p, _, _ := k.GetStringValue("Path")
		k.Close()
		for _, e := range splitPathList(p) {
			if samePathEntry(e, config.BinDir()) {
				onPath = true
			}
		}
	}
	fmt.Fprintf(w, "Bin dir on user PATH: %s\n", yesNo(onPath))

	autostart := false
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE); err == nil {
		_, _, err := k.GetStringValue(runValueName)
		autostart = err == nil
		k.Close()
	}
	fmt.Fprintf(w, "Login autostart:   %s\n", yesNo(autostart))
}

// dumpHostLogs tails lerd's own host-process logs, which Windows keeps as files
// under <DataDir>\logs instead of a journal.
func dumpHostLogs(w io.Writer, n int, filter *logFilter) {
	dumpWindowsLogs(w, filepath.Join(config.DataDir(), "logs"), n, filter)
}

func dumpWindowsLogs(w io.Writer, dir string, n int, filter *logFilter) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(w, "(no log directory at %s)\n", dir)
		return
	}
	var names []string
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".log")
		if !ok || e.IsDir() {
			continue
		}
		if isContentUnit(name) && !windowsHostLogs[name] {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		fmt.Fprintln(w, "(no lerd logs)")
		return
	}
	for _, name := range names {
		path := filepath.Join(dir, name+".log")
		fmt.Fprintf(w, "── %s (last %d lines)\n", path, n)
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(w, "(unreadable: %v)\n\n", err)
			continue
		}
		fmt.Fprintln(w, filter.clean(tailLines([]byte(decodeConsoleText(data)), n, 4000)))
		fmt.Fprintln(w)
	}
}

// dumpResolverConfig shows the NRPT rule that sends the lerd TLD to lerd-dns,
// Windows' counterpart of /etc/resolv.conf.
func dumpResolverConfig(w io.Writer) {
	tld := "test"
	if cfg, err := config.LoadGlobal(); err == nil && cfg.DNS.TLD != "" {
		tld = cfg.DNS.TLD
	}
	fmt.Fprintf(w, "── NRPT rules for .%s\n", tld)
	script := fmt.Sprintf(`Get-DnsClientNrptRule | Where-Object { $_.Namespace -like '*.%s' } | ForEach-Object { "$($_.Namespace) -> $($_.NameServers -join ', ')" }`, tld)
	out, _, err := runProbe(exec.CommandContext, "powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShell(script))
	switch text := strings.TrimSpace(decodeConsoleText(out)); {
	case err != nil:
		fmt.Fprintf(w, "(failed: %v)\n", err)
	case text == "":
		fmt.Fprintln(w, "(no rule: .test names will not reach lerd-dns)")
	default:
		fmt.Fprintln(w, text)
	}
}

type probeCmd func(ctx context.Context, name string, args ...string) *exec.Cmd

func podmanProbe(ctx context.Context, name string, args ...string) *exec.Cmd {
	return podman.CmdContext(ctx, append([]string{name}, args...)...)
}

// runProbe runs one bounded diagnostic command and returns stdout and stderr
// separately, so a warning on stderr does not corrupt parsed output.
func runProbe(mk probeCmd, name string, args ...string) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), reportProbeTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := mk(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("timed out after %s", reportProbeTimeout)
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

func probeError(err error, stderr []byte) string {
	if msg := strings.TrimSpace(string(stderr)); msg != "" {
		return msg
	}
	if err != nil {
		return err.Error()
	}
	return "no output"
}

// decodeConsoleText turns console output into a string. wsl.exe and msiexec
// logs are UTF-16LE, often without a BOM; anything else is taken as UTF-8.
func decodeConsoleText(b []byte) string {
	isUTF16 := len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE
	if isUTF16 {
		b = b[2:]
	} else if len(b) >= 2 && len(b)%2 == 0 {
		zeros := 0
		for i := 1; i < len(b); i += 2 {
			if b[i] == 0 {
				zeros++
			}
		}
		isUTF16 = zeros*2 >= len(b)/2
	}
	s := string(b)
	if isUTF16 {
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(b[2*i:])
		}
		s = string(utf16.Decode(u))
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\x00", ""), "\r\n", "\n")
}

func formatGiB(b uint64) string {
	const gib = 1 << 30
	if b%gib == 0 {
		return fmt.Sprintf("%d GiB", b/gib)
	}
	return fmt.Sprintf("%.1f GiB", float64(b)/gib)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func enabledOrNot(b bool) string {
	if b {
		return "enabled"
	}
	return "not enabled"
}

func installedOrNot(b bool) string {
	if b {
		return "installed"
	}
	return "not installed"
}

func presentOrNot(b bool) string {
	if b {
		return "present"
	}
	return "not present"
}

func existsOrMissing(path string) string {
	if _, err := os.Stat(path); err != nil {
		return "missing"
	}
	return "present"
}
