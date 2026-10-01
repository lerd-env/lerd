//go:build windows

package services

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// windowsServiceManager is the Windows counterpart of the systemd and launchd
// managers. There is no init system to hand units to, so a unit is a JSON
// definition under <DataDir>/units. Container units are `podman run -d` argv
// (podman's own --restart policy does the supervision); service units are host
// processes started detached, with their pid recorded next to the definition
// and their output redirected to <DataDir>/logs/<name>.log.
type windowsServiceManager struct{}

const (
	kindService   = "service"
	kindContainer = "container"

	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
	createNoWindow        = 0x08000000
	stillActive           = 259
)

type unitDef struct {
	Kind    string          `json:"kind"`
	Args    []string        `json:"args"`
	Restart keepAlivePolicy `json:"restart"`
}

func init() {
	mgr := &windowsServiceManager{}
	Mgr = mgr
	podman.WriteContainerUnitFn = mgr.WriteContainerUnit
	podman.DaemonReloadFn = mgr.DaemonReload
	podman.SkipQuadletUpToDateCheck = true
	podman.UsePlatformUnitLifecycle(mgr)
	podman.RemoveContainerUnitFn = mgr.RemoveContainerUnit
	podman.AfterQuadletWriteFn = mgr.WriteContainerUnit
}

// --- Paths and state files ---

func unitsDir() string { return filepath.Join(config.DataDir(), "units") }

func logsDir() string { return filepath.Join(config.DataDir(), "logs") }

func defPath(name string) string { return filepath.Join(unitsDir(), name+".json") }

func pidPath(name string) string { return filepath.Join(unitsDir(), name+".pid") }

func loadDef(name string) (unitDef, error) {
	var d unitDef
	data, err := os.ReadFile(defPath(name))
	if err != nil {
		return d, err
	}
	return d, json.Unmarshal(data, &d)
}

// writeDef stores d and reports whether the file changed.
func writeDef(name string, d unitDef) (bool, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return false, err
	}
	if existing, err := os.ReadFile(defPath(name)); err == nil && string(existing) == string(data) {
		return false, nil
	}
	config.GuardRealWrite(defPath(name))
	if err := os.MkdirAll(unitsDir(), 0755); err != nil {
		return false, err
	}
	if err := os.MkdirAll(logsDir(), 0755); err != nil {
		return false, err
	}
	return true, os.WriteFile(defPath(name), data, 0644)
}

func removeDef(name string) error {
	config.GuardRealWrite(defPath(name))
	_ = os.Remove(pidPath(name))
	if err := os.Remove(defPath(name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func readPID(name string) int {
	data, err := os.ReadFile(pidPath(name))
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return n
}

// --- Process helpers ---

func findPowerShell() (string, error) { return exec.LookPath("powershell.exe") }

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h) //nolint:errcheck
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// killTree ends pid and everything it spawned. Windows has no SIGTERM for a
// windowless process, and workers fork php children that must not be orphaned.
func killTree(pid int) {
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

// spawn starts args in a hidden console of their own, not DETACHED_PROCESS: a
// console program such as PowerShell or node exits at once with no console to
// attach to. Output is appended to the
// unit's log, and records the pid.
func spawn(name string, args []string) error {
	logf, err := os.OpenFile(filepath.Join(logsDir(), name+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer logf.Close() //nolint:errcheck
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | createNoWindow,
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", name, err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	config.GuardRealWrite(pidPath(name))
	return os.WriteFile(pidPath(name), []byte(strconv.Itoa(pid)), 0644)
}

// --- Service unit files ---

func (m *windowsServiceManager) WriteServiceUnit(name, content string) error {
	_, err := m.WriteServiceUnitIfChanged(name, content)
	return err
}

func (m *windowsServiceManager) WriteServiceUnitIfChanged(name, content string) (bool, error) {
	args, keepAlive, err := parseServiceUnit(name, rebaseLerdDirs(content))
	if err != nil {
		return false, err
	}
	return writeDef(name, unitDef{Kind: kindService, Args: args, Restart: keepAlive})
}

// Timers have no Windows counterpart yet, the same gap macOS has: scheduled
// workers are skipped rather than run as a restart loop.
func (m *windowsServiceManager) WriteTimerUnitIfChanged(_, _ string) (bool, error) {
	return false, nil
}

func (m *windowsServiceManager) RemoveTimerUnit(_ string) error { return nil }

func (m *windowsServiceManager) ListTimerUnits(_ string) []string { return nil }

func (m *windowsServiceManager) RemoveServiceUnit(name string) error { return removeDef(name) }

func (m *windowsServiceManager) ListServiceUnits(nameGlob string) []string {
	entries, _ := filepath.Glob(filepath.Join(unitsDir(), nameGlob+".json"))
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(filepath.Base(e), ".json"))
	}
	return names
}

// --- Container unit files ---

func (m *windowsServiceManager) WriteContainerUnit(name, content string) error {
	content = rebaseLerdDirs(content)
	var lanExposed, servicesExposed bool
	if cfg, err := config.LoadGlobal(); err == nil && cfg != nil {
		lanExposed, servicesExposed = cfg.LAN.Exposed, cfg.LAN.ServicesExposed
	}
	content = podman.BindQuadletForLAN(name, content, lanExposed, servicesExposed)
	// The Podman Machine's user-mode network handles publishes like gvproxy
	// on macOS: no dual-stack binds and no explicit IP on a privileged port.
	content = stripIPv6PublishPorts(content)

	c := parseSection(content, "Container")
	mapContainerPaths(c)
	args, err := containerToPodmanArgs(c)
	if err != nil {
		return fmt.Errorf("container unit %s: %w", name, err)
	}
	precreateBindMountDirs(c["Volume"])
	_, err = writeDef(name, unitDef{Kind: kindContainer, Args: args})
	return err
}

func (m *windowsServiceManager) ContainerUnitInstalled(name string) bool {
	_, err := os.Stat(defPath(name))
	return err == nil
}

// RemoveContainerUnit stops the container before dropping its definition, so a
// removed service does not keep running with nothing left describing it.
func (m *windowsServiceManager) RemoveContainerUnit(name string) error {
	_ = podman.StopUnit(name)
	return removeDef(name)
}

func (m *windowsServiceManager) ListContainerUnits(nameGlob string) []string {
	return m.ListServiceUnits(nameGlob)
}

// --- Lifecycle ---

func (m *windowsServiceManager) DaemonReload() error { return nil }

func (m *windowsServiceManager) Start(name string) error {
	d, err := loadDef(name)
	if err != nil {
		return fmt.Errorf("unit %s is not installed: %w", name, err)
	}
	if len(d.Args) == 0 {
		return fmt.Errorf("unit %s has no command", name)
	}
	if d.Kind == kindContainer {
		podmanStartSem <- struct{}{}
		rerr := runPodmanWithError(d.Args)
		<-podmanStartSem
		if rerr != nil {
			return fmt.Errorf("podman run %s: %w", name, rerr)
		}
		return nil
	}
	if pidAlive(readPID(name)) {
		return nil
	}
	return spawn(name, d.Args)
}

func (m *windowsServiceManager) Stop(name string) error {
	if running, _ := podman.ContainerRunning(name); running {
		podman.Cmd("stop", "-t", "5", name).Run() //nolint:errcheck
		podman.Cmd("rm", "-f", name).Run()        //nolint:errcheck
	}
	podman.MarkManagedWatcherStop(name)
	if pid := readPID(name); pid > 0 {
		if pidAlive(pid) {
			killTree(pid)
		}
		_ = os.Remove(pidPath(name))
	}
	return nil
}

func (m *windowsServiceManager) Restart(name string) error {
	if err := m.Stop(name); err != nil {
		return err
	}
	return m.Start(name)
}

// Enable and Disable follow the macOS convention: a unit that has a definition
// is enabled, so Enable starts it and Disable stops it.
func (m *windowsServiceManager) Enable(name string) error { return m.Start(name) }

func (m *windowsServiceManager) Disable(name string) error { return m.Stop(name) }

func (m *windowsServiceManager) IsActive(name string) bool {
	if podman.Cache.Running(name) {
		return true
	}
	return pidAlive(readPID(name))
}

func (m *windowsServiceManager) IsEnabled(name string) bool {
	_, err := os.Stat(defPath(name))
	return err == nil
}

func (m *windowsServiceManager) UnitStatus(name string) (string, error) {
	return m.unitStatus(name, nil)
}

// unitStatus maps process state onto systemd's vocabulary. A pid file whose
// process is gone means the unit died without Stop removing it, so it is
// failed rather than inactive. snapshot, when given, is a prefetched container
// state map so a whole sweep costs one podman query.
func (m *windowsServiceManager) unitStatus(name string, snapshot map[string]bool) (string, error) {
	if _, err := os.Stat(defPath(name)); err != nil {
		if containerUp(name, snapshot) {
			return "active", nil
		}
		return "unknown", nil
	}
	if pid := readPID(name); pid > 0 {
		if pidAlive(pid) {
			return "active", nil
		}
		return "failed", nil
	}
	if containerUp(name, snapshot) {
		return "active", nil
	}
	return "inactive", nil
}

func containerUp(name string, snapshot map[string]bool) bool {
	if snapshot != nil {
		return snapshot[name]
	}
	return podman.Cache.Running(name)
}

// AllUnitStates returns every lerd-* unit's state keyed by both "name" and
// "name.service", the form workerheal and the dashboard look up.
func (m *windowsServiceManager) AllUnitStates() map[string]string {
	snapshot := podman.Cache.Snapshot()
	if snapshot == nil {
		snapshot = map[string]bool{}
	}
	out := map[string]string{}
	for _, name := range m.ListServiceUnits("lerd-*") {
		state, _ := m.unitStatus(name, snapshot)
		if state == "" || state == "unknown" {
			continue
		}
		out[name] = state
		out[name+".service"] = state
	}
	return out
}

// InstalledUnitBinary returns the program the installed unit runs, or "" when
// it has no definition, so callers can tell a live binary from a stale one.
func InstalledUnitBinary(name string) string {
	d, err := loadDef(name)
	if err != nil || len(d.Args) == 0 {
		return ""
	}
	return d.Args[0]
}
