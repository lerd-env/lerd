package ui

import (
	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/podman"
)

// Seams for the native side so the routing can be tested without launchd.
var (
	nativeEnsureFn = nativephp.Ensure
	nativeStopFn   = nativephp.Stop
)

// startPHPVersion brings up whatever serves a PHP version: the launchd pool on
// the host under the native runtime, the shared FPM container otherwise.
// Acting on the container under native reported success while changing nothing
// that was running.
func startPHPVersion(native bool, version string) error {
	if native {
		return nativeEnsureFn(version)
	}
	return podman.StartUnit(podman.FPMUnitName(version))
}

// stopPHPVersion is startPHPVersion's counterpart.
func stopPHPVersion(native bool, version string) error {
	if native {
		return nativeStopFn(version)
	}
	return podman.StopUnit(podman.FPMUnitName(version))
}
