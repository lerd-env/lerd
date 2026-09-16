package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The restart the toggle reports, and the hint it gives when that fails, have to
// name what actually serves requests. Under the native runtime that is the host
// pool: naming an FPM unit sends the reader to a systemctl that macOS does not
// have, for a container the runtime never built.
func TestXdebugRestartTarget(t *testing.T) {
	native := xdebugRestartTarget("8.5", config.PHPRuntimeNative)
	if strings.Contains(native, "lerd-php") {
		t.Errorf("native target must not name a container unit: %q", native)
	}
	if !strings.Contains(native, "8.5") {
		t.Errorf("native target should name the version: %q", native)
	}

	container := xdebugRestartTarget("8.5", config.PHPRuntimeContainer)
	if container != "lerd-php85-fpm" {
		t.Errorf("container target = %q, want lerd-php85-fpm", container)
	}
}

// Restarting the per-site containers on a version is a container-runtime step;
// under native those sites are served by the host pool the toggle just reloaded.
func TestXdebugTouchesSiteContainersOnlyInContainerMode(t *testing.T) {
	if xdebugRestartsSiteContainers(config.PHPRuntimeNative) {
		t.Error("native runtime must not restart per-site containers")
	}
	if !xdebugRestartsSiteContainers(config.PHPRuntimeContainer) {
		t.Error("container runtime must restart per-site containers")
	}
}
