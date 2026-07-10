package siteinfo

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// unitCacheTTL bounds how long a batched systemctl snapshot is reused before
// a refresh is triggered on the next lookup.
const unitCacheTTL = 3 * time.Second

// UnitMeta carries the per-unit properties the reachability probe needs beyond
// the active state: when the unit last entered active (wall-clock, derived from
// systemd's monotonic timestamp so no locale-dependent string is parsed) and its
// WorkingDirectory (a worktree worker's checkout path). Zero values mean the
// property was unavailable this tick; callers treat that as "unknown" and fall
// back to the process-only behaviour.
type UnitMeta struct {
	ActiveEnter time.Time
	WorkingDir  string
}

type unitCache struct {
	mu     sync.Mutex
	states map[string]string
	meta   map[string]UnitMeta
	at     time.Time
}

var (
	globalUnitCache unitCache

	// unitCacheListFn is swappable for tests. It returns the raw output of
	// `systemctl --user list-units --all --no-legend --plain 'lerd-*'`.
	unitCacheListFn = defaultUnitCacheList

	// unitShowFn is swappable for tests. Given the discovered unit names it
	// returns the raw output of `systemctl --user show -p Id -p
	// ActiveEnterTimestampMonotonic -p WorkingDirectory <units...>`. The state
	// path stays on list-units (unchanged, proven); this only adds the two
	// extra properties the reachability probe needs.
	unitShowFn = defaultUnitShow

	// allUnitStatesFn lets non-systemd platforms override the enumeration
	// entirely. When non-nil it bypasses unitCacheListFn and returns the
	// unit→state map directly. Set from unitcache_darwin.go's init() to
	// route through podman.UnitLifecycle (launchd-backed on macOS).
	allUnitStatesFn func() map[string]string

	// allUnitMetaFn is the darwin override for AllUnitMeta, mirroring
	// allUnitStatesFn. nil on Linux, where the systemctl path fills meta.
	allUnitMetaFn func() map[string]UnitMeta

	// invalidateExtraFn clears any platform-specific TTL cache layered on
	// top of allUnitStatesFn. Set from unitcache_darwin.go to drop the
	// launchd-states cache; nil on Linux where the systemctl path uses the
	// shared globalUnitCache directly.
	invalidateExtraFn func()
)

func defaultUnitCacheList() (string, error) {
	out, err := exec.Command("systemctl", "--user", "list-units", "--all", "--no-legend", "--plain", "lerd-*").Output()
	return string(out), err
}

// defaultUnitShow batches one `systemctl show` over the discovered unit names to
// read the two properties the reachability probe needs. Mirrors the pattern in
// stats.showProps.
func defaultUnitShow(units []string) (string, error) {
	if len(units) == 0 {
		return "", nil
	}
	args := append([]string{"--user", "show", "-p", "Id", "-p", "ActiveEnterTimestampMonotonic", "-p", "WorkingDirectory"}, units...)
	out, err := exec.Command("systemctl", args...).Output()
	return string(out), err
}

// systemBootTime derives the wall-clock time the machine booted from
// /proc/uptime, used to turn systemd's monotonic ActiveEnter into a wall-clock
// timestamp comparable to a file mtime. ok is false when uptime is unreadable.
func systemBootTime() (time.Time, bool) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return time.Time{}, false
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return time.Time{}, false
	}
	sec, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Now().Add(-time.Duration(sec * float64(time.Second))), true
}

// parseUnitMeta turns `systemctl show` output (blank-line-separated per-unit
// blocks labelled by Id) into a unit→UnitMeta map. ActiveEnterTimestampMonotonic
// is microseconds since boot; 0 (never active) yields a zero ActiveEnter. bootOK
// false leaves ActiveEnter zero, so the gate that reads it simply doesn't fire.
func parseUnitMeta(raw string, boot time.Time, bootOK bool) map[string]UnitMeta {
	out := make(map[string]UnitMeta)
	var id string
	var m UnitMeta
	flush := func() {
		if id != "" {
			out[id] = m
			if strings.HasSuffix(id, ".service") {
				out[strings.TrimSuffix(id, ".service")] = m
			}
		}
		id, m = "", UnitMeta{}
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "Id":
			id = val
		case "WorkingDirectory":
			m.WorkingDir = val
		case "ActiveEnterTimestampMonotonic":
			if bootOK {
				if usec, err := strconv.ParseUint(val, 10, 64); err == nil && usec > 0 {
					m.ActiveEnter = boot.Add(time.Duration(usec) * time.Microsecond)
				}
			}
		}
	}
	flush()
	return out
}

// InvalidateUnitCache forces the next UnitStatus lookup to re-run systemctl.
// Call this after any mutation that changes lerd-* unit state (start, stop,
// enable, disable, etc.) so cached "active" values do not go stale. Also
// invalidates any platform-specific cache (launchd states on darwin).
func InvalidateUnitCache() {
	globalUnitCache.mu.Lock()
	globalUnitCache.at = time.Time{}
	globalUnitCache.mu.Unlock()
	if invalidateExtraFn != nil {
		invalidateExtraFn()
	}
}

// AllUnitStates returns a snapshot of every cached lerd-* unit state
// (unit name → "active" | "inactive" | "failed" | …). The map is a copy
// safe for callers to walk without holding the cache mutex. Triggers a
// refresh if the cache is stale, but otherwise reuses the same batched
// systemctl snapshot the dashboard's enrichment path is already populating
// — zero extra subprocess cost for callers like the worker-health detector.
func AllUnitStates() map[string]string {
	if allUnitStatesFn != nil {
		return allUnitStatesFn()
	}
	globalUnitCache.mu.Lock()
	defer globalUnitCache.mu.Unlock()
	if globalUnitCache.states == nil || time.Since(globalUnitCache.at) > unitCacheTTL {
		_ = globalUnitCache.refreshLocked()
	}
	out := make(map[string]string, len(globalUnitCache.states))
	for k, v := range globalUnitCache.states {
		out[k] = v
	}
	return out
}

// AllUnitStatesOK is AllUnitStates plus a trust signal: ok is false when the
// batched systemctl enumeration was attempted this call and failed, leaving the
// snapshot empty or stale. Callers that infer "unit absent -> removed" must check
// ok first, since a failed enumeration makes every unit look absent. On the
// non-systemd override path (darwin) ok is always true.
func AllUnitStatesOK() (map[string]string, bool) {
	if allUnitStatesFn != nil {
		return allUnitStatesFn(), true
	}
	globalUnitCache.mu.Lock()
	defer globalUnitCache.mu.Unlock()
	ok := true
	if globalUnitCache.states == nil || time.Since(globalUnitCache.at) > unitCacheTTL {
		if err := globalUnitCache.refreshLocked(); err != nil {
			ok = false
		}
	}
	out := make(map[string]string, len(globalUnitCache.states))
	for k, v := range globalUnitCache.states {
		out[k] = v
	}
	return out, ok
}

// AllUnitMeta returns a snapshot of the per-unit ActiveEnter + WorkingDirectory
// filled from the same batched refresh AllUnitStates uses, so the reachability
// probe (worktree path + the mtime gate) reads one source. On the non-systemd
// override path (darwin) it returns whatever the launchd walker can supply.
func AllUnitMeta() map[string]UnitMeta {
	if allUnitMetaFn != nil {
		return allUnitMetaFn()
	}
	globalUnitCache.mu.Lock()
	defer globalUnitCache.mu.Unlock()
	if globalUnitCache.states == nil || time.Since(globalUnitCache.at) > unitCacheTTL {
		_ = globalUnitCache.refreshLocked()
	}
	out := make(map[string]UnitMeta, len(globalUnitCache.meta))
	for k, v := range globalUnitCache.meta {
		out[k] = v
	}
	return out
}

// unitStatusCached returns the active state of a lerd-* unit, consulting a
// short-lived batched snapshot. One systemctl call populates ~all lerd units
// instead of one subprocess per worker.
func unitStatusCached(name string) (string, error) {
	globalUnitCache.mu.Lock()
	defer globalUnitCache.mu.Unlock()

	if globalUnitCache.states == nil || time.Since(globalUnitCache.at) > unitCacheTTL {
		if err := globalUnitCache.refreshLocked(); err != nil {
			return "unknown", nil
		}
	}

	if st, ok := globalUnitCache.states[name]; ok {
		return st, nil
	}
	return "unknown", nil
}

func (c *unitCache) refreshLocked() error {
	out, err := unitCacheListFn()
	if err != nil {
		return err
	}
	states := make(map[string]string, 64)
	var serviceNames []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Columns: UNIT LOAD ACTIVE SUB DESCRIPTION
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		unit, active := fields[0], fields[2]
		// systemctl reports "not-found active running" when the unit file
		// is gone but a container from an earlier load still owns the
		// cgroup; normalise non-"loaded" LOAD values so we don't go green.
		if fields[1] != "loaded" {
			active = "inactive"
		}
		// Strip the .service suffix so callers can pass either form.
		// Timer and other suffixes are preserved since enrichWorkers
		// explicitly looks up "lerd-schedule-<site>.timer".
		states[unit] = active
		if strings.HasSuffix(unit, ".service") {
			states[strings.TrimSuffix(unit, ".service")] = active
			serviceNames = append(serviceNames, unit)
		}
	}
	c.states = states

	// Second batched call for the two probe properties (ActiveEnter + WorkingDir).
	// A failure here leaves meta empty, which callers treat as "unknown" and fall
	// back to process-only behaviour, so the state path above is never affected.
	c.meta = map[string]UnitMeta{}
	if raw, err := unitShowFn(serviceNames); err == nil {
		boot, bootOK := systemBootTime()
		c.meta = parseUnitMeta(raw, boot, bootOK)
	}

	c.at = time.Now()
	return nil
}
