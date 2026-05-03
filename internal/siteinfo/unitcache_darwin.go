//go:build darwin

package siteinfo

import "github.com/geodro/lerd/internal/podman"

func init() {
	// On macOS, workers are managed by launchd + podman containers — there is
	// no systemd. Override the default unitStatusFn (which calls systemctl) so
	// that worker status is queried through the darwinServiceManager instead,
	// which checks launchd state and the running podman container directly.
	unitStatusFn = podman.UnitStatus

	// Stub out the legacy systemctl list-units path. AllUnitStates routes
	// through podman.UnitLifecycle below so this fallback is never reached.
	unitCacheListFn = func() (string, error) { return "", nil }

	// Plug the launchd plist walker (implemented in services/launchd_darwin.go
	// on darwinServiceManager) into AllUnitStates so cross-platform callers —
	// worker-heal Detect, dashboard banner, MCP workers_health — see real
	// failed-unit state instead of an empty map.
	allUnitStatesFn = func() map[string]string {
		if podman.UnitLifecycle == nil {
			return map[string]string{}
		}
		return podman.UnitLifecycle.AllUnitStates()
	}
}
