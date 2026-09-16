package cli

import (
	"errors"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A plain FPM site's restart bounced lerd-php<ver>-fpm whatever the runtime.
// Under native that container does not exist, so the command reached for an
// image nothing had built and failed against a registry, which is not an answer
// to "restart my site".
func TestRestartPlainFPMFollowsTheRuntime(t *testing.T) {
	prevUnit, prevPool := restartFPMContainer, restartNativePool
	t.Cleanup(func() { restartFPMContainer, restartNativePool = prevUnit, prevPool })

	var units, pools []string
	restartFPMContainer = func(u string) error { units = append(units, u); return nil }
	restartNativePool = func(v string) error { pools = append(pools, v); return nil }

	target, err := restartPlainFPM("shop", "8.5", config.PHPRuntimeNative)
	if err != nil {
		t.Fatalf("native restart: %v", err)
	}
	if len(pools) != 1 || pools[0] != "8.5" {
		t.Errorf("native must restart the host pool, got %v", pools)
	}
	if len(units) != 0 {
		t.Errorf("native must not touch a container, got %v", units)
	}
	if target == "lerd-php85-fpm" {
		t.Errorf("native must not report a container name, got %q", target)
	}

	units, pools = nil, nil
	target, err = restartPlainFPM("shop", "8.5", config.PHPRuntimeContainer)
	if err != nil {
		t.Fatalf("container restart: %v", err)
	}
	if len(units) != 1 || units[0] != "lerd-php85-fpm" {
		t.Errorf("container path wrong, got %v", units)
	}
	if len(pools) != 0 {
		t.Errorf("container must not restart a host pool, got %v", pools)
	}
	if target != "lerd-php85-fpm" {
		t.Errorf("target = %q, want the container name", target)
	}
}

func TestRestartPlainFPMSurfacesTheNativeFailure(t *testing.T) {
	prev := restartNativePool
	t.Cleanup(func() { restartNativePool = prev })
	restartNativePool = func(string) error { return errors.New("launchctl said no") }

	if _, err := restartPlainFPM("shop", "8.5", config.PHPRuntimeNative); err == nil {
		t.Error("a failed pool restart must be reported")
	}
}
