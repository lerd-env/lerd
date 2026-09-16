package xdebugops

import (
	"errors"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Under the native runtime there is no FPM container to restart: the toggle has
// to kick the version's host pool instead. It used to write a quadlet and then
// fail against an image that was never built, leaving the ini live but the
// running pool on the old settings while reporting success.
func TestApplyRestartTargetFollowsTheRuntime(t *testing.T) {
	prevQuadlet, prevUnit, prevPool := writeFPMQuadlet, restartFPMUnit, reloadNativePool
	t.Cleanup(func() {
		writeFPMQuadlet, restartFPMUnit, reloadNativePool = prevQuadlet, prevUnit, prevPool
	})

	var quadlets, units, pools []string
	writeFPMQuadlet = func(v string) error { quadlets = append(quadlets, v); return nil }
	restartFPMUnit = func(u string) error { units = append(units, u); return nil }
	reloadNativePool = func(v string) error { pools = append(pools, v); return nil }

	res, err := applyRestart("8.5", config.PHPRuntimeNative)
	if err != nil || !res.Restarted {
		t.Fatalf("native restart: res=%+v err=%v", res, err)
	}
	if len(pools) != 1 || pools[0] != "8.5" {
		t.Errorf("native must reload the host pool, got %v", pools)
	}
	if len(units) != 0 || len(quadlets) != 0 {
		t.Errorf("native must not touch the container: units=%v quadlets=%v", units, quadlets)
	}

	quadlets, units, pools = nil, nil, nil
	res, err = applyRestart("8.5", config.PHPRuntimeContainer)
	if err != nil || !res.Restarted {
		t.Fatalf("container restart: res=%+v err=%v", res, err)
	}
	if len(quadlets) != 1 || len(units) != 1 || units[0] != "lerd-php85-fpm" {
		t.Errorf("container path wrong: quadlets=%v units=%v", quadlets, units)
	}
	if len(pools) != 0 {
		t.Errorf("container must not reload a host pool, got %v", pools)
	}
}

// A failed reload is reported rather than swallowed, the same as the container path.
func TestApplyRestartReportsNativeFailure(t *testing.T) {
	prevPool := reloadNativePool
	t.Cleanup(func() { reloadNativePool = prevPool })
	reloadNativePool = func(string) error { return errors.New("launchctl said no") }

	res, err := applyRestart("8.5", config.PHPRuntimeNative)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Restarted || res.RestartErr == nil {
		t.Errorf("a failed reload must surface: %+v", res)
	}
}
