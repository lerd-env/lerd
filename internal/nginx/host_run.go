package nginx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// Up is Running for status surfaces polled often: the host's pid file, or the
// container cache rather than a podman call per read.
func Up() bool {
	if OnHost() {
		return hostMasterPID() > 0
	}
	return podman.Cache.Running(podman.WebServerUnit)
}

// Running reports whether nginx is up, the host's or its container.
func Running() bool {
	running, _ := containerRunningFn(podman.WebServerUnit)
	return running
}

func hostConfPath() string { return filepath.Join(config.NginxDir(), "nginx.conf") }

// hostNginxCmd runs the host's nginx on lerd's config, prefixed at lerd's nginx
// directory so `include fastcgi_params` and friends resolve beside nginx.conf.
func hostNginxCmd(ctx context.Context, args ...string) (*exec.Cmd, error) {
	bin, ok := hostNginxBinary()
	if !ok {
		return nil, fmt.Errorf("nginx is not installed on the host: brew install nginx")
	}
	return exec.CommandContext(ctx, bin, append([]string{"-c", hostConfPath(), "-p", config.NginxDir()}, args...)...), nil
}

// hostNginx runs the host's nginx once and returns what it printed.
func hostNginx(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd, err := hostNginxCmd(ctx, args...)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err = cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

// HostServiceUnit is the lerd-nginx unit for the host's nginx. services.Mgr
// writes it as a launchd plist; nginx.conf keeps it in the foreground for launchd.
func HostServiceUnit() string {
	bin, _ := hostNginxBinary()
	return "[Unit]\nDescription=Lerd nginx\nAfter=network.target\n\n" +
		"[Service]\nExecStart=" + bin + " -c " + hostConfPath() + " -p " + config.NginxDir() + "\nRestart=always\n\n" +
		"[Install]\nWantedBy=default.target\n"
}

func hostPidFile() string { return filepath.Join(config.RunDir(), "nginx.pid") }

// hostMasterPID is the running master's pid, 0 when none runs.
func hostMasterPID() int {
	b, err := os.ReadFile(hostPidFile())
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 || syscall.Kill(pid, 0) != nil {
		return 0
	}
	return pid
}

// hostRunning reads the master's pid file; nginx writes it on start and removes
// it on a clean stop, and a pid nothing answers to is a stopped nginx.
func hostRunning() (running, known bool) { return hostMasterPID() > 0, true }

// hostWorkerPIDs lists the workers of the running master.
func hostWorkerPIDs() ([]string, error) {
	master := hostMasterPID()
	if master == 0 {
		return nil, ErrNotRunning
	}
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(master)).Output()
	if err != nil && len(out) == 0 {
		return nil, nil
	}
	return strings.Fields(string(out)), nil
}

// WriteServiceUnit writes a unit through the service manager, set by the binary's
// entry point rather than imported: that package's init makes every quadlet write
// a real plist, which no test of this package may do.
var WriteServiceUnit func(name, content string) (bool, error)

// Seams for writing the unit and dropping the container, which a test must not
// do to the developer's own lerd.
var (
	writeServiceUnitFn = func(name, content string) (bool, error) {
		if WriteServiceUnit == nil {
			return false, fmt.Errorf("no service manager to write %s with", name)
		}
		return WriteServiceUnit(name, content)
	}
	removeContainerFn = func(name string) { _, _ = podman.Run("rm", "-f", name) }
)

// writeHostUnit makes lerd-nginx the host's nginx: the unit replaces the
// container's under the same name, and the container goes, since it holds the
// loopback port the host's nginx listens on. The caller restarts the unit.
func writeHostUnit() (bool, error) {
	changed, err := writeServiceUnitFn(podman.WebServerUnit, HostServiceUnit())
	if err != nil {
		return false, err
	}
	removeContainerFn(podman.WebServerUnit)
	// The container's quadlet goes too: a pass over every lerd-*.container (the LAN
	// rebind) would otherwise rewrite it and its launchd job over the host's unit.
	quadlet := filepath.Join(config.QuadletDir(), podman.WebServerUnit+".container")
	if err := os.Remove(quadlet); err != nil && !os.IsNotExist(err) {
		return changed, err
	}
	return changed, nil
}
