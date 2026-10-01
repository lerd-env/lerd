//go:build darwin

package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// launchctl runs a launchctl command with a 15-second timeout so a throttled
// or unresponsive service can never hang lerd indefinitely.
var launchctl = func(args ...string) ([]byte, error) {
	if len(args) > 0 && launchctlMutates[args[0]] {
		config.GuardRealLaunchd(strings.Join(args, " "))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "launchctl", args...).CombinedOutput()
}

// launchctlMutates lists the verbs that change the user's launchd domain. Reads
// like list and print are harmless, so only these are guarded under test.
var launchctlMutates = map[string]bool{
	"bootstrap": true, "bootout": true, "enable": true,
	"disable": true, "kickstart": true, "load": true, "unload": true,
}

// bootout removes a job from the launchd domain. Every bootout of the watcher
// goes through here so it is always marked as a lerd-initiated stop first:
// launchd delivers a bootout as the same SIGTERM a logout does, and the watcher
// tears the whole environment down when it reads one as a logout.
func bootout(name, domain, label string) ([]byte, error) {
	podman.MarkManagedWatcherStop(name)
	return launchctl("bootout", domain+"/"+label)
}

// uidDomain returns the launchd GUI domain for the current user, e.g. "gui/501".
func uidDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func init() {
	mgr := &darwinServiceManager{}
	Mgr = mgr
	// Override service-manager hooks to use launchd instead of systemd.
	podman.WriteContainerUnitFn = mgr.WriteContainerUnit
	podman.DaemonReloadFn = mgr.DaemonReload
	podman.SkipQuadletUpToDateCheck = true
	// Bind the concrete type so UnitLifecycle picks up AllUnitStates, which
	// isn't part of services.ServiceManager (Linux has no need for it — the
	// systemctl batched-list path covers Linux callers).
	podman.UsePlatformUnitLifecycle(mgr)
	podman.RemoveContainerUnitFn = mgr.RemoveContainerUnit
	// Keep launchd plists in sync when WriteQuadletDiff updates a .container file.
	podman.AfterQuadletWriteFn = func(name, content string) error {
		return mgr.WriteContainerUnit(name, content)
	}
}

// plistArgs parses the ProgramArguments array from a plist file and returns
// the argument strings. Used to run container units directly from Go code
// rather than via launchctl kickstart, so we control launch concurrency.
func plistArgs(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Find the ProgramArguments array using simple string search.
	s := string(data)
	const key = "<key>ProgramArguments</key>"
	idx := strings.Index(s, key)
	if idx < 0 {
		return nil, fmt.Errorf("ProgramArguments not found in %s", path)
	}
	s = s[idx+len(key):]
	start := strings.Index(s, "<array>")
	end := strings.Index(s, "</array>")
	if start < 0 || end < 0 {
		return nil, fmt.Errorf("ProgramArguments array not found in %s", path)
	}
	block := s[start+len("<array>") : end]
	var args []string
	for {
		open := strings.Index(block, "<string>")
		close := strings.Index(block, "</string>")
		if open < 0 || close < 0 {
			break
		}
		args = append(args, xmlUnescStr(block[open+len("<string>"):close]))
		block = block[close+len("</string>"):]
	}
	return args, nil
}

type darwinServiceManager struct{}

// --- Path helpers ---

func launchAgentsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents")
}

// launchAgentsDirFn and containerSnapshotFn are the seams the whole-sweep path
// uses, so tests can point it at a fixture directory and count how many times
// container state is queried.
var (
	launchAgentsDirFn   = launchAgentsDir
	containerSnapshotFn = func() map[string]bool { return podman.Cache.Snapshot() }
)

// containerRunning answers "is this unit's container up?" from a snapshot taken
// once for a whole sweep, falling back to a per-unit lookup when no snapshot was
// prefetched. In a process without the container cache running (every CLI
// invocation, and the MCP server) the per-unit path is a `podman inspect`
// subprocess, which on macOS is a round trip into the podman VM.
func containerRunning(name string, snapshot map[string]bool) bool {
	if snapshot != nil {
		return snapshot[name]
	}
	return podman.Cache.Running(name)
}

func lerdLogsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Logs", "lerd")
}

func plistPath(name string) string {
	return filepath.Join(launchAgentsDir(), name+".plist")
}

// writePlist and removePlist are the only ways a unit file is created or
// deleted, so the guard that keeps a forgetful test off the developer's own
// LaunchAgents dir sits on the single path all of them go through.
func writePlist(name, plist string) error {
	config.GuardRealWrite(plistPath(name))
	return os.WriteFile(plistPath(name), []byte(plist), 0644)
}

func removePlist(name string) error {
	config.GuardRealWrite(plistPath(name))
	if err := os.Remove(plistPath(name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func plistLabel(name string) string {
	return "com.lerd." + name
}

// --- Plist generation ---

func xmlEscStr(s string) string {
	// Only escape characters that are truly unsafe in XML text nodes.
	// xml.EscapeText also escapes ' → &#39; and " → &#34;, but Apple's plist
	// parser passes those numeric character references through literally
	// rather than decoding them, corrupting env var values like X_FRAME_OPTIONS = ''
	// into invalid Python. Single and double quotes are valid in XML PCDATA
	// without escaping.
	var buf strings.Builder
	for _, c := range s {
		switch c {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		default:
			buf.WriteRune(c)
		}
	}
	return buf.String()
}

// xmlUnescStr reverses xmlEscStr, so args read back out of a plist reach podman
// as they were written. &amp; is decoded last, otherwise an escaped "&amp;lt;"
// would collapse into a literal "<".
func xmlUnescStr(s string) string {
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	return strings.ReplaceAll(s, "&amp;", "&")
}

// watcherExitTimeout is how long launchd lets the logout teardown run before it
// SIGKILLs the watcher. The containers stop first, and a database declares up to
// 60s so it can finish writing, so a grace sized for them alone is already gone
// when the Podman Machine stop starts, which is the step whose loss is the whole
// point of the teardown. This covers that stop plus the 90s a machine stop gets
// elsewhere, with room to spare.
const watcherExitTimeout = 180

func buildPlist(lbl string, args []string, runAtLoad bool, keepAlive keepAlivePolicy, stdoutPath, stderrPath string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>`)
	sb.WriteString(xmlEscStr(lbl))
	sb.WriteString("</string>\n\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, a := range args {
		sb.WriteString("\t\t<string>")
		sb.WriteString(xmlEscStr(a))
		sb.WriteString("</string>\n")
	}
	sb.WriteString("\t</array>\n")
	if runAtLoad {
		sb.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	}
	switch keepAlive {
	case keepAliveAlways:
		sb.WriteString("\t<key>KeepAlive</key>\n\t<true/>\n")
	case keepAliveOnFailure:
		sb.WriteString("\t<key>KeepAlive</key>\n\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>\n\t</dict>\n")
	}
	if stdoutPath != "" {
		sb.WriteString("\t<key>StandardOutPath</key>\n\t<string>")
		sb.WriteString(xmlEscStr(stdoutPath))
		sb.WriteString("</string>\n")
	}
	if stderrPath != "" {
		sb.WriteString("\t<key>StandardErrorPath</key>\n\t<string>")
		sb.WriteString(xmlEscStr(stderrPath))
		sb.WriteString("</string>\n")
	}
	// The watcher runs the full teardown (containers, then the Podman Machine
	// VM) when launchd signals a logout, which does not fit the 5s grace
	// launchd gives these jobs before SIGKILL. Every other job stops fast, and
	// a longer timeout there would only slow down a hung unit.
	if lbl == plistLabel(podman.WatcherUnit) {
		sb.WriteString(fmt.Sprintf("\t<key>ExitTimeOut</key>\n\t<integer>%d</integer>\n", watcherExitTimeout))
	}
	if ownsPodmanMachine(lbl) {
		sb.WriteString("\t<key>AbandonProcessGroup</key>\n\t<true/>\n")
	}
	sb.WriteString("</dict>\n</plist>\n")
	return sb.String()
}

// machineOwningUnits are the host jobs that can bring the Podman Machine up:
// lerd-autostart and lerd-tray by running `lerd start`, lerd-ui in-process from
// the dashboard, and lerd-watcher when it remounts stale container storage.
var machineOwningUnits = []string{"lerd-autostart", "lerd-ui", "lerd-tray", podman.WatcherUnit}

// ownsPodmanMachine reports whether a job needs AbandonProcessGroup. vfkit and
// gvproxy reparent to init but keep the process group of whatever started them,
// and launchd's default is to kill what remains in a job's group once the job
// exits, taking the VM with it. Container and worker jobs never start the VM
// and want that cleanup, so the key stays scoped to the units above.
func ownsPodmanMachine(lbl string) bool {
	for _, unit := range machineOwningUnits {
		if lbl == plistLabel(unit) {
			return true
		}
	}
	return false
}

func ensurePlistDirs(name string) error {
	if err := os.MkdirAll(launchAgentsDir(), 0755); err != nil {
		return err
	}
	return os.MkdirAll(lerdLogsDir(), 0755)
}

func (m *darwinServiceManager) WriteServiceUnit(name, content string) error {
	args, keepAlive, err := parseServiceUnit(name, content)
	if err != nil {
		return err
	}
	if err := ensurePlistDirs(name); err != nil {
		return err
	}
	logPath := filepath.Join(lerdLogsDir(), name+".log")
	plist := buildPlist(plistLabel(name), args, true, keepAlive, logPath, logPath)
	return writePlist(name, plist)
}

func (m *darwinServiceManager) WriteServiceUnitIfChanged(name, content string) (bool, error) {
	args, keepAlive, err := parseServiceUnit(name, content)
	if err != nil {
		return false, err
	}

	logPath := filepath.Join(lerdLogsDir(), name+".log")
	newPlist := buildPlist(plistLabel(name), args, true, keepAlive, logPath, logPath)

	if existing, err := os.ReadFile(plistPath(name)); err == nil && string(existing) == newPlist {
		return false, nil
	}
	if err := ensurePlistDirs(name); err != nil {
		return false, err
	}
	return true, writePlist(name, newPlist)
}

// WriteTimerUnitIfChanged is a no-op on macOS until launchd
// StartCalendarInterval support is added. Scheduled framework workers
// (like Laravel 10's `schedule:run`) currently log a warning and skip
// on macOS rather than restart-loop as long-running daemons.
func (m *darwinServiceManager) WriteTimerUnitIfChanged(_, _ string) (bool, error) {
	return false, nil
}

// RemoveTimerUnit is a no-op on macOS — see WriteTimerUnitIfChanged.
func (m *darwinServiceManager) RemoveTimerUnit(_ string) error { return nil }

// ListTimerUnits returns no entries on macOS until launchd
// StartCalendarInterval support lands.
func (m *darwinServiceManager) ListTimerUnits(_ string) []string { return nil }

func (m *darwinServiceManager) RemoveServiceUnit(name string) error {
	return removePlist(name)
}

func (m *darwinServiceManager) ListServiceUnits(nameGlob string) []string {
	pattern := filepath.Join(launchAgentsDir(), nameGlob+".plist")
	entries, _ := filepath.Glob(pattern)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(filepath.Base(e), ".plist"))
	}
	return names
}

// --- Container unit files ---

func (m *darwinServiceManager) WriteContainerUnit(name, content string) error {
	// Apply the same unit-aware LAN policy as the Linux quadlet writer.
	lanExposed := false
	servicesExposed := false
	if cfg, err := config.LoadGlobal(); err == nil && cfg != nil {
		lanExposed = cfg.LAN.Exposed
		servicesExposed = cfg.LAN.ServicesExposed
	}
	content = podman.BindQuadletForLAN(name, content, lanExposed, servicesExposed)
	// gvproxy (macOS) cannot bind two specific host IPs on the same port;
	// drop IPv6 PublishPort lines so only IPv4 bindings reach podman run.
	content = stripIPv6PublishPorts(content)

	c := parseSection(content, "Container")
	args, err := containerToPodmanArgs(c)
	if err != nil {
		return fmt.Errorf("container unit %s: %w", name, err)
	}

	precreateBindMountDirs(c["Volume"])

	if err := ensurePlistDirs(name); err != nil {
		return err
	}
	logPath := filepath.Join(lerdLogsDir(), name+".log")
	// RunAtLoad=false: container units are started by `lerd start` (via lerd-autostart),
	// which first ensures Podman Machine is running. Firing podman run at login before
	// the machine is up causes silent failures, so we let lerd-autostart sequence it.
	// Stdout is suppressed (/dev/null) because `podman run -d` only prints the container
	// ID there; real container output is accessible via `podman logs <name>`.
	plist := buildPlist(plistLabel(name), args, false, keepAliveNever, "/dev/null", logPath)
	return writePlist(name, plist)
}

func (m *darwinServiceManager) ContainerUnitInstalled(name string) bool {
	_, err := os.Stat(plistPath(name))
	return err == nil
}

// RemoveContainerUnit takes the job out of the launchd domain before dropping its
// plist. Removing only the file leaves launchd holding a job whose plist is gone,
// which is how a removed service went on answering launchctl list forever. It
// goes through StopUnit rather than booting out here so the stop stays on the one
// funnel that already refuses to reach a real launchd from a test.
func (m *darwinServiceManager) RemoveContainerUnit(name string) error {
	_ = podman.StopUnit(name)
	return removePlist(name)
}

func (m *darwinServiceManager) ListContainerUnits(nameGlob string) []string {
	// Container units share the same plist directory; no separate extension.
	// We use the same glob pattern as service units — callers are expected to
	// pass a glob that uniquely identifies containers (e.g. "lerd-*").
	return m.ListServiceUnits(nameGlob)
}

// --- Service lifecycle ---

// DaemonReload is a no-op on macOS; launchd picks up plist changes on bootstrap.
func (m *darwinServiceManager) DaemonReload() error { return nil }

// bootstrap registers and starts the service plist in the user's GUI domain.
// If already bootstrapped, it kicks (restarts) the service instead.
func (m *darwinServiceManager) Start(name string) error {
	p := plistPath(name)
	if _, err := os.Stat(p); err != nil {
		return fmt.Errorf("plist not found for %s", name)
	}
	domain := uidDomain()
	label := plistLabel(name)

	// If the service is already in the domain, bootout first so the subsequent
	// bootstrap always picks up the current plist on disk. kickstart -k would
	// restart the job but launchd would use its cached plist, missing any
	// changes written by WriteServiceUnit / WriteContainerUnit.
	alreadyInDomain := false
	if _, err := launchctl("print", domain+"/"+label); err == nil {
		alreadyInDomain = true
		bootout(name, domain, label) //nolint:errcheck
		// Brief pause so macOS Sequoia+ doesn't reject the immediately-following
		// bootstrap with a spurious "already bootstrapped" (36) or EBUSY (5) error.
		time.Sleep(200 * time.Millisecond)
	}

	// Enable AFTER bootout — on macOS Ventura+, bootout marks the service as
	// disabled in launchd's persistent database, causing the next bootstrap to
	// fail with exit 5. Re-enabling here ensures bootstrap always succeeds.
	launchctl("enable", domain+"/"+label) //nolint:errcheck

	out, err := launchctl("bootstrap", domain, p)
	if err != nil {
		s := string(out)
		// 36 = already bootstrapped; "Bootstrap failed: 5" = EBUSY / I-O error
		// (macOS Ventura+ race after a rapid bootout+bootstrap) — both mean the
		// job is already in the domain, kick it to (re)start with the current plist.
		if strings.Contains(s, "36") || strings.Contains(s, "Bootstrap failed: 5") ||
			strings.Contains(s, "already bootstrapped") ||
			strings.Contains(s, "service already loaded") {
			// Already in domain — run container directly if it's a container unit.
			content2, _ := os.ReadFile(p)
			if !strings.Contains(string(content2), "<key>RunAtLoad</key>") {
				if args, aerr := plistArgs(p); aerr == nil && len(args) > 0 {
					podmanStartSem <- struct{}{}
					rerr := runPodmanWithError(args)
					<-podmanStartSem
					if rerr != nil {
						return fmt.Errorf("podman run %s: %w", name, rerr)
					}
					return nil
				}
			}
			// kickstart -k kills the running job first, which reaches the
			// watcher as the same SIGTERM a logout does. Nothing booted it
			// out on this path (print said it wasn't in the domain), so the
			// mark has to happen here or a start tears the environment down.
			podman.MarkManagedWatcherStop(name)
			if kout, kerr := launchctl("kickstart", "-k", domain+"/"+label); kerr != nil {
				ks := string(kout)
				// 37 = EALREADY — job is already running, treat as success.
				if strings.Contains(ks, "37") || strings.Contains(ks, "already running") {
					return nil
				}
				return fmt.Errorf("launchctl kickstart %s: %w\n%s", name, kerr, kout)
			}
			return nil
		}
		// If bootstrap failed and we just did a bootout, retry once — launchd on
		// Sequoia can transiently reject a re-bootstrap immediately after bootout.
		if alreadyInDomain {
			time.Sleep(300 * time.Millisecond)
			launchctl("enable", domain+"/"+label) //nolint:errcheck
			if out2, err2 := launchctl("bootstrap", domain, p); err2 != nil {
				return fmt.Errorf("launchctl bootstrap %s: %w\n%s", name, err2, out2)
			}
			return nil
		}
		return fmt.Errorf("launchctl bootstrap %s: %w\n%s", name, err, out)
	}
	// Container units use RunAtLoad=false so bootstrap alone doesn't start them.
	// Service units use RunAtLoad=true so bootstrap already started them — no kick needed.
	content, _ := os.ReadFile(p)
	if strings.Contains(string(content), "<key>RunAtLoad</key>") {
		// RunAtLoad is supposed to start it, but launchd does not always
		// oblige: after an abrupt teardown the job loads and stays at "not
		// running", and returning here reported a start that never happened.
		return ensureStarted(name, unitRunning, kickstartUnit)
	}
	// Container unit: run podman directly (with concurrency limit) instead of
	// launchctl kickstart. kickstart lets launchd fire all podman run processes
	// simultaneously, which overwhelms the Podman Machine SSH connection when
	// N services start in parallel. Running directly lets us gate on podmanStartSem.
	args, err := plistArgs(p)
	if err != nil || len(args) == 0 {
		// Fallback to kickstart if plist parsing fails.
		launchctl("kickstart", domain+"/"+label) //nolint:errcheck
		return nil
	}
	podmanStartSem <- struct{}{}
	rerr := runPodmanWithError(args)
	<-podmanStartSem
	if rerr != nil {
		return fmt.Errorf("podman run %s: %w", name, rerr)
	}
	return nil
}

// Stop removes the service from the user's GUI domain (bootout) and also stops
// any detached podman container running under the same name. The podman stop is
// needed because container units use -d (detached) + --restart=always, so the
// container keeps running independently of launchd after the plist is booted out.
func (m *darwinServiceManager) Stop(name string) error {
	// Stop and remove the container only if it is actually running.
	// Skipping the podman calls when the container is absent avoids flooding the
	// Podman Machine SSH socket with N parallel no-op requests during lerd stop.
	if running, _ := podman.ContainerRunning(name); running {
		podman.Cmd("stop", "-t", "5", name).Run() //nolint:errcheck
		podman.Cmd("rm", "-f", name).Run()        //nolint:errcheck
	}

	domain := uidDomain()
	label := plistLabel(name)

	out, err := bootout(name, domain, label)
	if err != nil {
		s := string(out)
		// 36 = not loaded / already gone — treat as success
		if strings.Contains(s, "36") || strings.Contains(s, "No such process") ||
			strings.Contains(s, "Could not find") || strings.Contains(s, "not bootstrapped") {
			return nil
		}
		return fmt.Errorf("launchctl bootout %s: %w\n%s", name, err, out)
	}
	return nil
}

// Restart kicks the service if loaded, otherwise bootstraps it fresh.
func (m *darwinServiceManager) Restart(name string) error {
	// For container units, the detached podman container runs independently
	// of launchd. Stop it explicitly so the restart is clean even if
	// --replace is ever removed from the podman run args.
	if running, _ := podman.ContainerRunning(name); running {
		podman.Cmd("stop", "-t", "5", name).Run() //nolint:errcheck
		podman.Cmd("rm", "-f", name).Run()        //nolint:errcheck
	}

	domain := uidDomain()
	label := plistLabel(name)

	// Bootout so the subsequent Start (bootstrap) picks up the current
	// plist on disk. kickstart -k would use launchd's cached copy.
	if _, err := launchctl("print", domain+"/"+label); err == nil {
		bootout(name, domain, label) //nolint:errcheck
		time.Sleep(200 * time.Millisecond)
	}
	return m.Start(name)
}

// Enable marks the service as enabled (persists across logins) and bootstraps it.
func (m *darwinServiceManager) Enable(name string) error {
	domain := uidDomain()
	label := plistLabel(name)

	// enable records the intent; bootstrap actually starts it now
	launchctl("enable", domain+"/"+label) //nolint:errcheck
	return m.Start(name)
}

// Disable stops the service and marks it disabled so it won't start at login.
func (m *darwinServiceManager) Disable(name string) error {
	domain := uidDomain()
	label := plistLabel(name)

	_ = m.Stop(name)
	launchctl("disable", domain+"/"+label) //nolint:errcheck
	return nil
}

// IsActive returns true if the service is currently running.
// For container units we also check the container directly.
func (m *darwinServiceManager) IsActive(name string) bool {
	if podman.Cache.Running(name) {
		return true
	}
	domain := uidDomain()
	label := plistLabel(name)
	out, err := launchctl("print", domain+"/"+label)
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "state = running")
}

// IsEnabled returns true if the plist exists in LaunchAgents.
// On macOS, placing a plist in ~/Library/LaunchAgents is the equivalent of "enabled".
func (m *darwinServiceManager) IsEnabled(name string) bool {
	_, err := os.Stat(plistPath(name))
	return err == nil
}

// UnitStatus returns a status string similar to systemd's active state.
// Container units (podman run -d) exit immediately with code 0 once the
// container is detached, so we fall back to checking whether the container
// is actually running rather than trusting launchd's "state = waiting/exited".
func (m *darwinServiceManager) UnitStatus(name string) (string, error) {
	return m.unitStatus(name, nil)
}

// unitStatus is UnitStatus with an optional prefetched container snapshot, so a
// whole-directory sweep resolves every unit's container state from one query
// instead of one per unit.
func (m *darwinServiceManager) unitStatus(name string, snapshot map[string]bool) (string, error) {
	domain := uidDomain()
	label := plistLabel(name)
	out, err := launchctl("print", domain+"/"+label)
	if err != nil {
		// Not loaded at all — check container directly before giving up.
		if containerRunning(name, snapshot) {
			return "active", nil
		}
		if _, statErr := os.Stat(plistPath(name)); statErr == nil {
			return "inactive", nil
		}
		return "unknown", nil
	}
	s := string(out)
	if strings.Contains(s, "state = running") {
		return "active", nil
	}
	// For exited-0 or waiting: the job may be a container launcher that
	// succeeded (-d detach). Check the actual container state.
	if containerRunning(name, snapshot) {
		return "active", nil
	}
	// Universal failure signal: an explicit non-zero last exit code is
	// "this is broken" regardless of whether the plist is a container
	// launcher (mysql, postgres, …) or a runtime-mode worker (queue,
	// schedule, horizon — `/bin/sh worker.sh` → `podman exec ... php
	// artisan ...`). Without this, runtime workers between retries would
	// fall through to the state=waiting/exit=0 branch and surface as
	// "inactive" even though the previous run aborted with exit != 0.
	if hasNonZeroExitCode(s) {
		return "failed", nil
	}
	// Container units that exited cleanly (last exit code = 0) but whose
	// detached container isn't currently running are crashed post-detach —
	// `podman run -d` returned 0, the container died after, --restart=always
	// can't bring it back (data dir / image / port issue). Treat as failed
	// so workerheal.Detect picks them up. Skip when the launcher hasn't
	// completed yet ("(never exited)"): that's the brief window between
	// Start() returning and ContainerCache picking up the new state, and
	// reporting "failed" there would be a false positive that could trigger
	// spurious heal cycles on every fresh start.
	if isContainerPlist(out) && strings.Contains(s, "last exit code = 0") {
		return "failed", nil
	}
	if strings.Contains(s, "state = waiting") || strings.Contains(s, "last exit code = 0") {
		return "inactive", nil
	}
	return "failed", nil
}

// hasNonZeroExitCode reports whether the launchctl print output has a
// "last exit code = N" line where N is neither 0 nor "(never exited)".
// Returns false when the field is absent so newly-bootstrapped units that
// haven't run yet aren't misreported as failed.
func hasNonZeroExitCode(s string) bool {
	const key = "last exit code = "
	idx := strings.Index(s, key)
	if idx < 0 {
		return false
	}
	rest := s[idx+len(key):]
	end := strings.IndexByte(rest, '\n')
	if end < 0 {
		end = len(rest)
	}
	val := strings.TrimSpace(rest[:end])
	if val == "" || val == "0" || val == "(never exited)" {
		return false
	}
	return true
}

// isContainerPlist reports whether the launchctl print output describes a
// container-unit plist (i.e. the launcher exec'd `podman run`). Runtime-mode
// workers launch via `/bin/sh worker.sh` so the launchctl-visible program
// path doesn't include `/podman`; the embedded `podman exec` call lives
// inside the script and is invisible here. Used by UnitStatus to
// differentiate container units that crashed post-detach from runtime
// workers in transient inactive states.
func isContainerPlist(out []byte) bool {
	s := string(out)
	return strings.Contains(s, "/podman") && strings.Contains(s, "run")
}

// AllUnitStates enumerates every lerd-* plist in ~/Library/LaunchAgents and
// returns a snapshot keyed by unit name → systemd-style state string. Both
// "lerd-foo" and "lerd-foo.service" forms are populated so cross-platform
// callers (workerheal, dashboard banner) can use a single suffix-based lookup.
//
// This is the launchd analogue of `systemctl --user list-units lerd-*` and
// is wired onto siteinfo.AllUnitStates from siteinfo/unitcache_darwin.go.
func (m *darwinServiceManager) AllUnitStates() map[string]string {
	pattern := filepath.Join(launchAgentsDirFn(), "lerd-*.plist")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return map[string]string{}
	}
	// One container query for the whole sweep. Resolving liveness per unit
	// instead costs a `podman inspect` each in any process without the cache
	// running, which is what made a single MCP worker health call take about a
	// second on a 25 worker install.
	snapshot := containerSnapshotFn()
	if snapshot == nil {
		snapshot = map[string]bool{}
	}
	out := make(map[string]string, len(matches)*2)
	for _, path := range matches {
		name := strings.TrimSuffix(filepath.Base(path), ".plist")
		if !strings.HasPrefix(name, "lerd-") {
			continue
		}
		state, _ := m.unitStatus(name, snapshot)
		if state == "" || state == "unknown" {
			continue
		}
		out[name] = state
		out[name+".service"] = state
	}
	return out
}

// ensureStarted kicks a job that bootstrap left loaded but idle, and leaves a
// running one alone so a healthy worker is never restarted for nothing. The
// launchctl calls are injected so the decision is testable without a domain.
func ensureStarted(name string, running func(string) bool, kick func(string) error) error {
	if running(name) {
		return nil
	}
	if err := kick(name); err != nil {
		return fmt.Errorf("launchctl kickstart %s: %w", name, err)
	}
	return nil
}

// unitRunning reports whether launchd has the job running rather than merely
// loaded.
func unitRunning(name string) bool {
	out, err := launchctl("print", uidDomain()+"/"+plistLabel(name))
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "state = running")
}

func kickstartUnit(name string) error {
	out, err := launchctl("kickstart", uidDomain()+"/"+plistLabel(name))
	if err != nil && !strings.Contains(string(out), "already running") {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// launchd hands lerd a restricted PATH, so a bare ExecStart command falls back
// to the usual Homebrew and system locations.
func init() {
	fallbackBinDirs = []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"}
}
