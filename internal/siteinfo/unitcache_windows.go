//go:build windows

package siteinfo

import (
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/podman"
)

// windowsUnitStates caches the service manager's state sweep for the same 3s
// the Linux path holds `systemctl list-units` for. One sweep reads every unit's
// pid file and costs a single podman query for the containers, and the
// dashboard asks for each site's workers in turn on every render.
type windowsUnitStates struct {
	mu     sync.Mutex
	states map[string]string
	at     time.Time
}

var windowsUnitStatesCache windowsUnitStates

const windowsUnitStatesTTL = 3 * time.Second

// windowsAllUnitStates returns a copy of the cached sweep, refreshing it when
// it is older than the TTL.
func windowsAllUnitStates() map[string]string {
	if podman.UnitLifecycle == nil {
		return map[string]string{}
	}
	windowsUnitStatesCache.mu.Lock()
	defer windowsUnitStatesCache.mu.Unlock()
	if windowsUnitStatesCache.states == nil || time.Since(windowsUnitStatesCache.at) > windowsUnitStatesTTL {
		windowsUnitStatesCache.states = podman.UnitLifecycle.AllUnitStates()
		windowsUnitStatesCache.at = time.Now()
	}
	out := make(map[string]string, len(windowsUnitStatesCache.states))
	for k, v := range windowsUnitStatesCache.states {
		out[k] = v
	}
	return out
}

func init() {
	// Windows has no systemd: without this the dashboard asked systemctl for
	// every worker's state, got nothing, and showed running workers as stopped.
	// Unit state comes from lerd's own service manager instead.
	unitStatusFn = func(name string) (string, error) {
		if st, ok := windowsAllUnitStates()[strings.TrimSuffix(name, ".service")]; ok {
			return st, nil
		}
		return "unknown", nil
	}
	unitCacheListFn = func() (string, error) { return "", nil }
	allUnitStatesFn = windowsAllUnitStates
	// The service manager records no working directory to read back, so there
	// is no per-unit metadata; an empty map keeps AllUnitMeta off systemctl.
	allUnitMetaFn = func() map[string]UnitMeta { return map[string]UnitMeta{} }
	invalidateExtraFn = func() {
		windowsUnitStatesCache.mu.Lock()
		windowsUnitStatesCache.at = time.Time{}
		windowsUnitStatesCache.states = nil
		windowsUnitStatesCache.mu.Unlock()
	}
}
