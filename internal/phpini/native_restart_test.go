package phpini

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// php:ini and xdebug:on only take effect once FPM reloads. Under the native
// runtime the container is stopped by design, so restarting it would be a
// silent no-op: the setting is written and nothing ever picks it up.
func TestRestartTargetFollowsTheRuntime(t *testing.T) {
	var container, native []string
	restart := func(v string) error { container = append(container, v); return nil }
	reload := func(v string) error { native = append(native, v); return nil }

	if err := restartForRuntime("8.4", config.PHPRuntimeContainer, restart, reload); err != nil {
		t.Fatalf("container: %v", err)
	}
	if len(container) != 1 || len(native) != 0 {
		t.Errorf("container mode must restart the container, got c=%v n=%v", container, native)
	}

	container, native = nil, nil
	if err := restartForRuntime("8.4", config.PHPRuntimeNative, restart, reload); err != nil {
		t.Fatalf("native: %v", err)
	}
	if len(native) != 1 || len(container) != 0 {
		t.Errorf("native mode must reload the host listener, got c=%v n=%v", container, native)
	}
}
