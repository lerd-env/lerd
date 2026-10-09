//go:build windows

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/p9share"
	"github.com/geodro/lerd/internal/podman"
)

// takeOverP9Shares swaps Podman's 9p server for `lerd p9-serve` on a running
// Hyper-V machine: stop lerd's containers, unmount the shares in the VM, stop
// Podman's server, start lerd's on the same hvsock services and mount again.
// Anything lerd does not recognise leaves Podman's server where it is, and a
// failed start puts it back.
func takeOverP9Shares(machine string) error {
	procs, err := listProcesses()
	if err != nil {
		return fmt.Errorf("listing processes: %w", err)
	}
	var podmanSrv *winProcess
	for i, p := range procs {
		if isLerdP9Serve(p.CommandLine) {
			return nil
		}
		if _, ok := server9pArgs(p.CommandLine); ok {
			podmanSrv = &procs[i]
		}
	}
	if podmanSrv == nil {
		return nil
	}
	args, _ := server9pArgs(podmanSrv.CommandLine)
	shares, pid, err := p9share.ParseServerArgs(args)
	if err != nil {
		return fmt.Errorf("leaving Podman's 9p server in place, its arguments changed: %w", err)
	}
	plan, err := remountPlan(machine, shares)
	if err != nil {
		return fmt.Errorf("leaving Podman's 9p server in place: %w", err)
	}

	stopLerdContainers()
	if err := vmRun(machine, p9share.UnmountScript(plan)); err != nil {
		return fmt.Errorf("leaving Podman's 9p server in place, unmounting failed: %w", err)
	}
	_ = killPID(podmanSrv.PID, syscall.SIGKILL)

	// The guard runs the server and, should it die while the machine is up,
	// starts it again and remounts; killPID below is a tree kill, so a failed
	// swap ends both.
	guardArgs := append([]string{"p9-guard", "--machine", machine, "--"}, p9share.ServerArgs(shares, pid)...)
	ours, err := startDetachedLogged("p9-serve", installedLerdExe(), guardArgs)
	if err == nil {
		err = remount(machine, plan)
	}
	if err == nil {
		return nil
	}
	if ours > 0 {
		_ = killPID(ours, syscall.SIGKILL)
	}
	if line, decErr := windows.DecomposeCommandLine(podmanSrv.CommandLine); decErr == nil && len(line) > 0 {
		_, _ = startDetachedLogged("podman-server9p", line[0], line[1:])
		_ = remount(machine, plan)
	}
	return fmt.Errorf("lerd's 9p server did not come up, Podman's is back in place: %w", err)
}

// remountPlan maps every share to the VM folder the machine config mounts it at.
func remountPlan(machine string, shares []p9share.Share) ([]p9share.Mount, error) {
	out, err := machineQuery("machine", "inspect", "--format", "{{.ConfigDir.Path}}", machine)
	if err != nil {
		return nil, fmt.Errorf("locating the machine config: %w", err)
	}
	mounts, err := p9share.ReadMachineMounts(filepath.Join(strings.TrimSpace(string(out)), machine+".json"))
	if err != nil {
		return nil, err
	}
	return p9share.Remounts(shares, mounts)
}

// remount mounts the shares again, retrying while the new server comes up.
func remount(machine string, plan []p9share.Mount) error {
	var err error
	for i := 0; i < 5; i++ {
		time.Sleep(time.Second)
		if err = vmRun(machine, p9share.MountScript(plan)); err == nil {
			return nil
		}
	}
	return err
}

func vmRun(machine, script string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := machineCmdContext(ctx, "machine", "ssh", machine, script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// stopLerdContainers stops the containers holding the shares open; the start
// that called the swap brings them back up.
func stopLerdContainers() {
	out, err := podman.Cmd("ps", "-q", "--filter", "name=^lerd-").Output()
	if err != nil {
		return
	}
	if ids := strings.Fields(string(out)); len(ids) > 0 {
		_ = podman.Cmd(append([]string{"stop", "-t", "5"}, ids...)...).Run()
	}
}

// startDetachedLogged starts exe outliving lerd, its output appended to a log
// under the data dir, and returns its PID once it has survived a moment.
func startDetachedLogged(name, exe string, args []string) (int, error) {
	logDir := filepath.Join(config.DataDir(), "logs")
	_ = os.MkdirAll(logDir, 0o755)
	logf, err := os.OpenFile(filepath.Join(logDir, name+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer logf.Close() //nolint:errcheck
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = detachedSysProcAttr()
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	time.Sleep(500 * time.Millisecond)
	if !processExists(pid) {
		return 0, fmt.Errorf("%s exited at once, see %s", name, logf.Name())
	}
	return pid, nil
}

// server9pArgs returns the arguments after `machine server9p` when line is
// Podman's 9p server.
func server9pArgs(line string) ([]string, bool) {
	args, err := windows.DecomposeCommandLine(line)
	if err != nil || len(args) < 3 || !strings.EqualFold(filepath.Base(args[0]), "podman.exe") {
		return nil, false
	}
	if args[1] != "machine" || args[2] != "server9p" {
		return nil, false
	}
	return args[3:], true
}

// isLerdP9Serve reports whether line is a running `lerd p9-serve`, or the
// `lerd p9-guard` that keeps one up.
func isLerdP9Serve(line string) bool {
	args, err := windows.DecomposeCommandLine(line)
	return err == nil && len(args) >= 2 && strings.EqualFold(filepath.Base(args[0]), "lerd.exe") && (args[1] == "p9-serve" || args[1] == "p9-guard")
}

type winProcess struct {
	PID         int    `json:"ProcessId"`
	CommandLine string `json:"CommandLine"`
}

// listProcesses returns the podman.exe and lerd.exe processes with their
// command lines, which only WMI exposes for other processes.
func listProcesses() ([]winProcess, error) {
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		`Get-CimInstance Win32_Process -Filter "Name='podman.exe' OR Name='lerd.exe'" | Select-Object ProcessId,CommandLine | ConvertTo-Json -Compress`).Output()
	if err != nil {
		return nil, err
	}
	return parseProcessList(out)
}

func parseProcessList(out []byte) ([]winProcess, error) {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, nil
	}
	if out[0] == '{' {
		var p winProcess
		err := json.Unmarshal(out, &p)
		return []winProcess{p}, err
	}
	var ps []winProcess
	return ps, json.Unmarshal(out, &ps)
}
