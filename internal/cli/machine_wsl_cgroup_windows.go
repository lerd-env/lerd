//go:build windows

package cli

import (
	"context"
	"errors"
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
// A WSL kernel too old for netavark's nftables rules stops with the update to
// run; other probe failures are left for the real container start to report.
func ensureWSLContainersRun() error {
	out, err := probeMachineContainer()
	if err == nil {
		return nil
	}
	if isOldWSLKernelNetworkFailure(out) {
		return oldWSLKernelError()
	}
	if !isMissingCgroupController(out) {
		return nil
	}
	feedback.Line("Switching the WSL machine to the cgroupfs manager (WSL 3 cgroup layout, podman#29749)…")
	if err := applyCgroupfsDropIn(); err != nil {
		return fmt.Errorf("applying the WSL cgroup workaround: %w", err)
	}
	out, err = probeMachineContainer()
	switch {
	case err == nil:
	case isOldWSLKernelNetworkFailure(out):
		return oldWSLKernelError()
	case isMissingCgroupController(out):
		return fmt.Errorf("containers still cannot start in the WSL machine after switching to cgroupfs (see podman#29749): %s", strings.TrimSpace(out))
	}
	return nil
}

// isOldWSLKernelNetworkFailure recognises netavark failing to load its
// nftables ruleset. WSL kernels before 6.18 lack NFT_FIB_INET, which the
// ruleset of netavark 2 needs, so no container on the machine gets a network.
func isOldWSLKernelNetworkFailure(out string) bool {
	return strings.Contains(out, "netavark") && strings.Contains(out, "nftables error")
}

func oldWSLKernelError() error {
	return errors.New("containers cannot get a network in the WSL machine: this WSL kernel is too old for Podman's nftables rules (it lacks NFT_FIB_INET). " +
		"Update WSL, restart it, then run lerd install again:\n\n" +
		"    wsl --update\n" +
		"    wsl --shutdown\n" +
		"    lerd install")
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
