//go:build windows

package cli

import (
	"errors"
	"strings"
	"testing"
)

const pidsErr = "Error: preparing container abc for attach: crun: controller `pids` is not available under /sys/fs/cgroup/non-systemd/machine.slice/libpod-abc.scope/container/cgroup.controllers: OCI runtime error"

func TestIsMissingCgroupController(t *testing.T) {
	if !isMissingCgroupController(pidsErr) {
		t.Error("the WSL 3 pids error was not recognised")
	}
	for _, out := range []string{"", "Error: image not known", "Error: no such file or directory"} {
		if isMissingCgroupController(out) {
			t.Errorf("%q is not a cgroup controller failure", out)
		}
	}
}

// stubWSLCgroup replaces the probe with one answering from outs in order and
// records whether the cgroupfs drop-in was applied.
func stubWSLCgroup(t *testing.T, outs []string, applyErr error) *int {
	t.Helper()
	oldProbe, oldApply := probeMachineContainer, applyCgroupfsDropIn
	applied := 0
	probeMachineContainer = func() (string, error) {
		out := outs[0]
		outs = outs[1:]
		if out == "" {
			return "", nil
		}
		return out, errors.New("exit status 126")
	}
	applyCgroupfsDropIn = func() error { applied++; return applyErr }
	t.Cleanup(func() { probeMachineContainer, applyCgroupfsDropIn = oldProbe, oldApply })
	return &applied
}

func TestEnsureWSLContainersRunLeavesAWorkingMachineAlone(t *testing.T) {
	applied := stubWSLCgroup(t, []string{""}, nil)
	if err := ensureWSLContainersRun(); err != nil || *applied != 0 {
		t.Errorf("err=%v applied=%d, want nil and no change", err, *applied)
	}
}

func TestEnsureWSLContainersRunSwitchesToCgroupfs(t *testing.T) {
	applied := stubWSLCgroup(t, []string{pidsErr, ""}, nil)
	if err := ensureWSLContainersRun(); err != nil || *applied != 1 {
		t.Errorf("err=%v applied=%d, want nil and one switch", err, *applied)
	}
}

// Any other probe failure is not this bug, and is left for the real container
// start to report rather than masked by a config change.
func TestEnsureWSLContainersRunIgnoresOtherFailures(t *testing.T) {
	applied := stubWSLCgroup(t, []string{"Error: something else"}, nil)
	if err := ensureWSLContainersRun(); err != nil || *applied != 0 {
		t.Errorf("err=%v applied=%d, want nil and no change", err, *applied)
	}
}

func TestEnsureWSLContainersRunReportsWhenTheSwitchDoesNotHelp(t *testing.T) {
	stubWSLCgroup(t, []string{pidsErr, pidsErr}, nil)
	err := ensureWSLContainersRun()
	if err == nil || !strings.Contains(err.Error(), "29749") {
		t.Errorf("err = %v, want one pointing at podman#29749", err)
	}
}

// Once the cgroup error is gone, a different probe failure is not this bug and
// must not block lerd start.
func TestEnsureWSLContainersRunAcceptsAnUnrelatedFailureAfterTheSwitch(t *testing.T) {
	applied := stubWSLCgroup(t, []string{pidsErr, "Error: crun: pivot_root: Device or resource busy"}, nil)
	if err := ensureWSLContainersRun(); err != nil || *applied != 1 {
		t.Errorf("err=%v applied=%d, want nil after one switch", err, *applied)
	}
}

func TestEnsureWSLContainersRunReportsAFailedSwitch(t *testing.T) {
	stubWSLCgroup(t, []string{pidsErr}, errors.New("ssh failed"))
	if err := ensureWSLContainersRun(); err == nil || !strings.Contains(err.Error(), "ssh failed") {
		t.Errorf("err = %v, want the apply failure", err)
	}
}

const nftErr = `Error: preparing container c91d for attach: netavark (exit code 1): nftables error: "nft" did not return successfully while applying ruleset:`

// An older WSL kernel lacks NFT_FIB_INET, which netavark 2 needs for every
// container network; no config change helps, only a newer WSL kernel.
func TestEnsureWSLContainersRunAsksForAWSLUpdateOnTheNftablesError(t *testing.T) {
	applied := stubWSLCgroup(t, []string{nftErr}, nil)
	err := ensureWSLContainersRun()
	if err == nil || *applied != 0 {
		t.Fatalf("err=%v applied=%d, want an error and no config change", err, *applied)
	}
	for _, want := range []string{"wsl --update", "wsl --shutdown", "lerd install"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not say %q", err, want)
		}
	}
}

func TestEnsureWSLContainersRunAsksForAWSLUpdateWhenNftablesFailsAfterTheSwitch(t *testing.T) {
	stubWSLCgroup(t, []string{pidsErr, nftErr}, nil)
	if err := ensureWSLContainersRun(); err == nil || !strings.Contains(err.Error(), "wsl --update") {
		t.Errorf("err=%v, want the WSL update guidance", err)
	}
}

func TestIsOldWSLKernelNetworkFailure(t *testing.T) {
	if !isOldWSLKernelNetworkFailure(nftErr) {
		t.Error("the netavark nftables error was not recognised")
	}
	for _, out := range []string{"", pidsErr, "netavark (exit code 1): Must provide a valid firewall backend, got iptables"} {
		if isOldWSLKernelNetworkFailure(out) {
			t.Errorf("%q is not the old kernel failure", out)
		}
	}
}
