//go:build windows

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
)

// WSL 3 places the machine's processes in a cgroup that does not delegate the
// pids controller, so with podman's systemd cgroup manager no container starts
// (podman#29749). Switching that machine to cgroupfs is the upstream workaround.
const (
	wslCgroupfsDropIn  = "/etc/containers/containers.conf.d/90-lerd-wsl-cgroupfs.conf"
	wslCgroupfsConf    = "[engine]\ncgroup_manager = \"cgroupfs\"\n"
	wslCgroupProbeTime = 90 * time.Second
)

// ensureWSLContainersRun checks that the WSL machine can start a container and
// applies the cgroupfs workaround when it fails on a missing cgroup controller.
// Other probe failures are left for the real container start to report.
func ensureWSLContainersRun() error {
	out, err := probeMachineContainer()
	if err == nil || !isMissingCgroupController(out) {
		return nil
	}
	feedback.Line("Switching the WSL machine to the cgroupfs manager (WSL 3 cgroup layout, podman#29749)…")
	if err := applyCgroupfsDropIn(); err != nil {
		return fmt.Errorf("applying the WSL cgroup workaround: %w", err)
	}
	if out, err := probeMachineContainer(); err != nil && isMissingCgroupController(out) {
		return fmt.Errorf("containers still cannot start in the WSL machine after switching to cgroupfs (see podman#29749): %s", strings.TrimSpace(out))
	}
	return nil
}

// isMissingCgroupController recognises crun refusing a container because a
// cgroup controller it needs is not delegated to the container's cgroup.
func isMissingCgroupController(out string) bool {
	return strings.Contains(out, "crun: controller") && strings.Contains(out, "is not available")
}

// probeMachineContainer starts a throwaway container on the machine's /usr,
// which holds bin/true and its libraries, so it needs no image and works before
// lerd has pulled any. The machine's / cannot be used: crun cannot pivot_root
// onto the root it is running from. A var so tests can answer for it.
var probeMachineContainer = func() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wslCgroupProbeTime)
	defer cancel()
	out, err := podman.CmdContext(ctx, "run", "--rm", "--rootfs", "/usr", "/bin/true").CombinedOutput()
	return string(out), err
}

// applyCgroupfsDropIn writes the cgroupfs drop-in inside the machine and
// restarts podman so the API picks it up. The file lives on the machine's
// disk, so it survives a machine restart. A var so tests can stand in for it.
var applyCgroupfsDropIn = func() error {
	name := selectedMachineName()
	if name == "" {
		return fmt.Errorf("no Podman machine found")
	}
	steps := []struct {
		cmd   string
		stdin string
	}{
		{"sudo mkdir -p /etc/containers/containers.conf.d", ""},
		{"sudo tee " + wslCgroupfsDropIn, wslCgroupfsConf},
		{"sudo systemctl restart podman.socket podman.service", ""},
	}
	for _, s := range steps {
		ctx, cancel := context.WithTimeout(context.Background(), wslCgroupProbeTime)
		cmd := machineCmdContext(ctx, "machine", "ssh", name, s.cmd)
		if s.stdin != "" {
			cmd.Stdin = strings.NewReader(s.stdin)
		}
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("%s: %w: %s", s.cmd, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
