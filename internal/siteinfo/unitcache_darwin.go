//go:build darwin

package siteinfo

import (
	"sync"
	"time"

	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/services"
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

// darwinUnitMeta throttles the working-directory enumeration on the same TTL.
// It is cheaper than the states walk (a glob and a read per guard script rather
// than a launchctl fork), but AllUnitMeta is called from inside a per-worktree
// loop, so an uncached read would rescan the whole directory once per worker.
type darwinUnitMeta struct {
	mu   sync.Mutex
	meta map[string]UnitMeta
	at   time.Time
}

var (
	darwinUnitStatesCache darwinUnitStates
	darwinUnitMetaCache   darwinUnitMeta
)

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
		darwinUnitStatesCache.states = nil
		darwinUnitStatesCache.mu.Unlock()
		darwinUnitMetaCache.mu.Lock()
		darwinUnitMetaCache.at = time.Time{}
		darwinUnitMetaCache.meta = nil
		darwinUnitMetaCache.mu.Unlock()
	}

	// launchd exposes no ActiveEnter, so the dial gate keeps falling back to
	// always dialing here. WorkingDirectory it does not expose either, but lerd
	// writes one into every worker's guard script, so that is where it is read
	// back from: without it a per-worktree unit resolves with an empty probe
	// path and an orphan whose checkout is gone is never pruned on macOS.
	allUnitMetaFn = func() map[string]UnitMeta {
		darwinUnitMetaCache.mu.Lock()
		defer darwinUnitMetaCache.mu.Unlock()
		if darwinUnitMetaCache.meta == nil || time.Since(darwinUnitMetaCache.at) > darwinUnitStatesTTL {
			dirs := services.WorkerWorkingDirs()
			meta := make(map[string]UnitMeta, len(dirs))
			for unit, dir := range dirs {
				meta[unit] = UnitMeta{WorkingDir: dir}
			}
			darwinUnitMetaCache.meta = meta
			darwinUnitMetaCache.at = time.Now()
		}
		out := make(map[string]UnitMeta, len(darwinUnitMetaCache.meta))
		for k, v := range darwinUnitMetaCache.meta {
			out[k] = v
		}
		return out
	}
}
