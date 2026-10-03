//go:build windows

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/hostpath"
	"github.com/geodro/lerd/internal/logcolor"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/services"
	"github.com/geodro/lerd/internal/unitlog"
)

// errHostWorkersWindows is why host workers (Vite, Mix) stay off on Windows: they
// run Node on the host, which has no Windows path yet.
var errHostWorkersWindows = errors.New("host workers are not available on Windows yet")

// writeWorkerUnitFile writes a framework worker's unit on Windows. The site is
// mounted at its /mnt/<drive> path inside the machine, so the worker runs there:
//
//   - exec mode (the default): a service unit whose ExecStart is `podman exec`
//     into the site's FPM container, as on Linux. The service manager runs it
//     under `lerd supervise`, which restarts it by its policy, and a stop clears
//     what is left in the container through killWorkerInContainer.
//   - container mode: one detached container per worker, from the FPM image.
//
// Scheduled and host workers are refused earlier by workerSupportedOnPlatform.
func writeWorkerUnitFile(unitName, label, siteName, sitePath, phpVersion, command, restart, schedule, fpmUnit, requiresUnit string, host bool) (bool, error) {
	_ = requiresUnit
	// Generation-boundary guard so every caller is covered (incl. the boot
	// restore path): every value below is a line of the unit, and a cloned
	// repo's .lerd.yaml can set the worker ones.
	if err := validateWorkerUnitFields(unitName, map[string]string{
		"command":     command,
		"label":       label,
		"restart":     restart,
		"schedule":    schedule,
		"site name":   siteName,
		"site path":   sitePath,
		"PHP version": phpVersion,
		"FPM unit":    fpmUnit,
	}); err != nil {
		return false, err
	}
	if host {
		return false, errHostWorkersWindows
	}
	if schedule != "" {
		feedback.Warn("worker %s has schedule=%q which is not yet supported on Windows — skipping", unitName, schedule)
		return false, nil
	}

	if cfg, _ := config.LoadGlobal(); cfg != nil && cfg.WorkerExecMode() == config.WorkerExecModeContainer {
		var unit string
		if site, _ := config.FindSite(siteName); site != nil && site.IsCustomContainer() {
			unit = buildWindowsContainerWorkerUnit(unitName, podman.CustomImageName(siteName), sitePath, command, restart, false, "")
		} else {
			unit = buildWindowsContainerWorkerUnit(unitName, "lerd-php"+strings.ReplaceAll(phpVersion, ".", "")+"-fpm:local", sitePath, command, restart, true, phpVersion)
		}
		if err := services.Mgr.WriteContainerUnit(unitName, unit); err != nil {
			return false, err
		}
		return true, podman.DaemonReloadFn()
	}

	container := resolveWorkerFPMUnit(siteName, phpVersion)
	unit := buildWindowsExecWorkerUnit(unitName, label, siteName, sitePath, installedLerdExe(), podman.PodmanBin(), container, command, restart)
	saveWorkerReap(unitName, workerReap{Container: container, Command: command, Dir: hostpath.ToVM(sitePath)})
	return services.Mgr.WriteServiceUnitIfChanged(unitName, unit)
}

// installedLerdExe is the lerd.exe a unit should run: the installed copy, not a
// build folder's that happened to write the unit during `lerd install`.
func installedLerdExe() string {
	installed := filepath.Join(config.BinDir(), "lerd.exe")
	if _, err := os.Stat(installed); err == nil {
		return installed
	}
	return config.LerdBinary()
}

// runWorkerExec is `lerd worker-exec`: it clears what the unit's worker left in
// the container, then runs args with this process's output and returns the
// exit code. Without the clearing, a restart (which ends only the host side
// podman exec) or a dropped machine connection would leave the old worker
// running beside the new one.
func runWorkerExec(unitName string, args []string) (int, error) {
	if data, err := os.ReadFile(workerReapPath(unitName)); err == nil {
		var r workerReap
		if json.Unmarshal(data, &r) == nil && r.Container != "" && r.Command != "" && r.Dir != "" {
			reapWorkerInContainer(r)
		}
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exitErr):
		return int(int32(exitErr.ExitCode())), nil
	default:
		return 0, err
	}
}

// workerReap is what a stop needs to end an exec worker inside its container.
// Ending the unit only ends the host side `podman exec`: podman reaches the
// machine over a connection that carries no signal to the process it started,
// so the worker would go on running in the container. Kept as a sidecar for
// the same reason the macOS .reap file is: the stop path knows only the unit.
type workerReap struct {
	Container string `json:"container"`
	Command   string `json:"command"`
	Dir       string `json:"dir"`
}

func workerReapPath(unitName string) string {
	return filepath.Join(config.RunDir(), "workers", unitName+".reap.json")
}

// saveWorkerReap is best-effort: without it a stop may leave the worker running
// in the container, which is no reason to refuse starting it.
func saveWorkerReap(unitName string, r workerReap) {
	data, _ := json.Marshal(r)
	path := workerReapPath(unitName)
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err == nil {
		err = os.WriteFile(path, data, 0o644)
	}
	if err != nil {
		feedback.Warn("worker %s: writing reap sidecar: %v (stop may leave it running in the container)", unitName, err)
	}
}

// reapWorkerInContainer ends the in-container processes running the worker's
// command from the site's directory; other sites share the container and run
// the same command, so the directory is what tells them apart.
var reapWorkerInContainer = func(r workerReap) {
	_ = podman.RunSilent("exec", r.Container, "sh", "-c", inContainerReapSnippet(r.Command, r.Dir))
}

// buildWindowsExecWorkerUnit renders the exec-mode service unit: `podman exec`
// into the site's container, run through `lerd worker-exec` so a start first
// clears the worker's leftovers. Executable paths are quoted since they may
// hold spaces, and the working directory is the site's path inside the
// machine, not its Windows path.
func buildWindowsExecWorkerUnit(unitName, label, siteName, sitePath, lerdBin, podmanBin, container, command, restart string) string {
	return fmt.Sprintf(`[Unit]
Description=Lerd %s (%s)

[Service]
Type=simple
Restart=%s
ExecStart=%s worker-exec --unit %s -- %s exec -w %s --env=LERD_SITE=%s%s %s%s %s
`, label, siteName, restart, podman.ShellQuote(lerdBin), unitName, podman.ShellQuote(podmanBin), podman.ShellQuote(hostpath.ToVM(sitePath)), siteName,
		workerExecEnvFlags(sitePath), workerColorArgs(), container, command)
}

// buildWindowsContainerWorkerUnit renders the container-mode quadlet. Sources
// stay Windows paths, which podman maps itself; the service manager turns the
// site's own mount and working directory into their /mnt/<drive> paths. A
// custom container site brings its own PHP config, so phpConfig is false and
// only the FPM image gets lerd's ini files.
func buildWindowsContainerWorkerUnit(unitName, image, sitePath, command, restart string, phpConfig bool, phpVersion string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Container]\nImage=%s\nContainerName=%s\nNetwork=lerd\n", image, unitName)
	fmt.Fprintf(&b, "Volume=%s:/etc/hosts:ro\n", config.ContainerHostsFile())
	fmt.Fprintf(&b, "Volume=%s:%s:rw\n", sitePath, sitePath)
	if phpConfig {
		fmt.Fprintf(&b, "Volume=%s:/usr/local/etc/php/conf.d/99-xdebug.ini:ro\n", config.PHPConfFile(phpVersion))
		fmt.Fprintf(&b, "Volume=%s:/usr/local/etc/php/conf.d/98-lerd-user.ini:ro\n", config.PHPUserIniFile(phpVersion))
		fmt.Fprintf(&b, "Volume=%s:/usr/local/etc/php/conf.d/95-lerd-shared.ini:ro\n", config.SharedIniFile())
	}
	fmt.Fprintf(&b, "PodmanArgs=--security-opt=label=disable\nWorkingDir=%s\n%sExec=%s\n\n[Service]\nRestart=%s\n",
		sitePath, logcolor.QuadletEnvLines(), command, restart)
	return b.String()
}

func workerLogHint(unitName string, host bool) string {
	if !host {
		if cfg, _ := config.LoadGlobal(); cfg != nil && cfg.WorkerExecMode() == config.WorkerExecModeContainer {
			return "podman logs -f " + unitName
		}
	}
	return unitlog.LogHint(unitName)
}

// removeWorkerExecArtifacts runs on every worker stop, after the unit is down:
// it ends what the exec worker left in its container and drops the sidecar.
func removeWorkerExecArtifacts(unitName string) {
	path := workerReapPath(unitName)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var r workerReap
	if json.Unmarshal(data, &r) == nil && r.Container != "" && r.Command != "" && r.Dir != "" {
		reapWorkerInContainer(r)
	}
	_ = os.Remove(path)
}

// restoreWorker rewrites a worker's unit during `lerd start` and starts it
// again, as on Linux.
func restoreWorker(siteName, sitePath, phpVersion, workerName string, w config.FrameworkWorker) {
	if ok, _ := workerSupportedOnPlatform(w); !ok {
		return
	}
	// Resolve the same way WorkerStartForSite does so a project opted into
	// auto-reload keeps its reload command across lerd start and reboots.
	command := resolveWorkerCommand(sitePath, workerName, w)
	command = withWorkerProxyPort(siteName, sitePath, workerName, w, command)
	command = devServerCommand(siteName, sitePath, workerName, command, w)

	fpmUnit := resolveWorkerFPMUnit(siteName, phpVersion)
	unitName, displaySite := workerNames(siteName, sitePath, workerName)
	restart := w.Restart
	if restart == "" {
		restart = "always"
	}
	label := w.Label
	if label == "" {
		label = workerName
	}
	changed, err := writeWorkerUnitFile(unitName, label, displaySite, sitePath, phpVersion, command, restart, w.Schedule, fpmUnit, requiredServiceUnit(sitePath, w), w.Host)
	if err != nil {
		feedback.Warn("writing worker unit %s: %v", unitName, err)
		return
	}
	if changed {
		if err := syncWorkerBootArming(unitName); err != nil {
			feedback.Warn("enable %s: %v", unitName, err)
		}
	}
}

// migrateWorkersOnModeChangeStreaming is a no-op on Windows: the dashboard does
// not offer the mode switch here, and a worker picks the configured mode up
// the next time it is started.
func migrateWorkersOnModeChangeStreaming(_, _ string, _ func(WorkerModePhaseEvent)) error {
	return nil
}
