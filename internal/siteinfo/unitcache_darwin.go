//go:build darwin

package siteinfo

import (
	"sync"
	"time"

	"github.com/geodro/lerd/internal/podman"
)

// darwinUnitStatesCache mirrors the 3s TTL the linux path enforces around
// `systemctl list-units`. AllUnitStates is called on every dashboard render
// AND every workerheal.Detect invocation, and on darwin each call shells out
// to `launchctl print` once per plist (N round-trips per call). Without this
// throttle a 25-worker install burns ~25 launchctl subprocesses per render.
type darwinUnitStates struct {
	mu     sync.Mutex
	states map[string]string
	at     time.Time
}

var darwinUnitStatesCache darwinUnitStates

const darwinUnitStatesTTL = 3 * time.Second

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
	// failed-unit state instead of an empty map. The result is cached for
	// darwinUnitStatesTTL to match the systemctl-list throttle on Linux.
	allUnitStatesFn = func() map[string]string {
		if podman.UnitLifecycle == nil {
			return map[string]string{}
		}
		darwinUnitStatesCache.mu.Lock()
		defer darwinUnitStatesCache.mu.Unlock()
		if darwinUnitStatesCache.states == nil || time.Since(darwinUnitStatesCache.at) > darwinUnitStatesTTL {
			darwinUnitStatesCache.states = podman.UnitLifecycle.AllUnitStates()
			darwinUnitStatesCache.at = time.Now()
		}
		out := make(map[string]string, len(darwinUnitStatesCache.states))
		for k, v := range darwinUnitStatesCache.states {
			out[k] = v
		}
		return out
	}
	invalidateExtraFn = func() {
		darwinUnitStatesCache.mu.Lock()
		darwinUnitStatesCache.at = time.Time{}
		darwinUnitStatesCache.mu.Unlock()
	}

	// launchd exposes neither ActiveEnter nor a per-unit WorkingDirectory cleanly,
	// so meta is empty on darwin: the gate falls back to always dialing and heal
	// keeps its process-only path (a darwin follow-up; the incident is systemd).
	allUnitMetaFn = func() map[string]UnitMeta { return map[string]UnitMeta{} }
}
