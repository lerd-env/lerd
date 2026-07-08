package podman

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/freeport"
)

// fpmPortsBindable is the port-bindability probe, swapped in tests so the shift
// guard can be exercised without binding real sockets. The unit lifecycle calls
// are seams for the same reason: the restart-rollback path is exercised without a
// live podman.
var (
	fpmPortsBindable    = freeport.Bindable
	fpmQuadletInstalled = QuadletInstalled
	fpmUnitStatus       = UnitStatus
	fpmWriteQuadlet     = WriteFPMQuadlet
	fpmRestartUnit      = RestartUnit
)

// setVersionFPMPorts writes (or clears) a version's FPM port overrides on cfg,
// pruning the map to nil when the last version is removed. Shared by the save and
// its rollback so both persist the port list the same way.
func setVersionFPMPorts(cfg *config.GlobalConfig, version string, ports []string) {
	if len(ports) == 0 {
		delete(cfg.PHP.FPMPorts, version)
		if len(cfg.PHP.FPMPorts) == 0 {
			cfg.PHP.FPMPorts = nil
		}
		return
	}
	if cfg.PHP.FPMPorts == nil {
		cfg.PHP.FPMPorts = map[string][]string{}
	}
	cfg.PHP.FPMPorts[version] = ports
}

// SetFPMPorts replaces the extra published ports for a PHP version's shared FPM
// container with the given "host:container" specs, shifting any requested host
// port that is already claimed (by a lerd service, another version's pool, or an
// external listener) to the next free port, then re-rendering and restarting the
// version's FPM unit when it is running. It returns the resolved list actually
// persisted, which may differ from specs where a port was shifted. Single entry
// point shared by the CLI, MCP, and Web UI so all three get identical validation,
// the shift guard, and the restart.
func SetFPMPorts(version string, specs []string) ([]string, error) {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return nil, fmt.Errorf("loading global config: %w", err)
	}
	// Prior ports for this version, so a failed restart can roll the whole change
	// back to the mapping that was actually serving.
	prevPorts := append([]string(nil), cfg.PHP.FPMPorts[version]...)

	// A port this version already publishes is not a collision with itself: the
	// running container holds it now but frees it on the restart below, so a probe
	// would wrongly report it busy. Exclude these from both the reserved set and the
	// bindability check so a plain re-save keeps every port where it is.
	own := map[int]bool{}
	for _, s := range cfg.PHP.FPMPorts[version] {
		if h := specHostPort(s); h > 0 {
			own[h] = true
		}
	}
	reserved := config.ReservedHostPorts()
	for h := range own {
		delete(reserved, h)
	}

	used := map[int]bool{}
	taken := func(p int) bool {
		if used[p] || reserved[p] {
			return true
		}
		if own[p] {
			return false
		}
		return !fpmPortsBindable(p)
	}

	resolved := make([]string, 0, len(specs))
	seen := map[string]bool{}
	requested := map[string]bool{}
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		host, container, err := parseFPMPortSpec(spec)
		if err != nil {
			return nil, err
		}
		// Collapse an exact-duplicate request before the shift, so re-adding a
		// mapping the batch already carries is a no-op rather than being pushed
		// onto a redundant extra host port.
		reqKey := fmt.Sprintf("%d:%d", host, container)
		if requested[reqKey] {
			continue
		}
		requested[reqKey] = true
		if taken(host) {
			host = freeport.FirstFree(host+1, taken)
			if host == 0 {
				return nil, fmt.Errorf("no free host port available for container port %d", container)
			}
		}
		mapping := fmt.Sprintf("%d:%d", host, container)
		if seen[mapping] {
			continue
		}
		seen[mapping] = true
		used[host] = true
		resolved = append(resolved, mapping)
	}

	setVersionFPMPorts(cfg, version, resolved)
	if err := config.SaveGlobal(cfg); err != nil {
		return nil, err
	}

	// The override is saved regardless; only (re)write and restart the unit for a
	// version whose FPM quadlet actually exists, so a save for a not-yet-installed
	// version doesn't resurrect a unit that would grab a host port at boot.
	unit := "lerd-php" + strings.ReplaceAll(version, ".", "") + "-fpm"
	if !fpmQuadletInstalled(unit) {
		return resolved, nil
	}
	if err := fpmWriteQuadlet(version); err != nil {
		return resolved, err
	}
	if status, _ := fpmUnitStatus(unit); status == "active" || status == "activating" {
		if err := fpmRestartUnit(unit); err != nil {
			return rollbackFPMPorts(version, unit, prevPorts, resolved, err)
		}
	}
	return resolved, nil
}

// rollbackFPMPorts restores a version's previous FPM ports after a restart on the
// new mapping failed (a port grabbed between the bindability probe and the restart
// can leave the shared FPM down). On a clean restore it returns the previous ports
// with an explanatory error; if the restore itself fails the unit may be down and
// the operator is told to run `lerd start`.
func rollbackFPMPorts(version, unit string, prevPorts, resolved []string, cause error) ([]string, error) {
	cfg, err := config.LoadGlobal()
	if err == nil && cfg != nil {
		setVersionFPMPorts(cfg, version, prevPorts)
		if config.SaveGlobal(cfg) == nil && fpmWriteQuadlet(version) == nil && fpmRestartUnit(unit) == nil {
			return prevPorts, fmt.Errorf("could not restart %s on the new ports, restored the previous ports: %w", unit, cause)
		}
	}
	return resolved, fmt.Errorf("could not restart %s and could not restore the previous ports, run `lerd start`: %w", unit, cause)
}

// AddFPMPort appends a single "host:container" mapping to a version's FPM ports,
// shifting the host port if it collides, and returns the resolved host port so
// the caller can report where it actually landed.
func AddFPMPort(version string, host, container int) (int, error) {
	if host < 1 || host > 65535 || container < 1 || container > 65535 {
		return 0, fmt.Errorf("invalid port mapping %d:%d: each port must be 1-65535", host, container)
	}
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return 0, fmt.Errorf("loading global config: %w", err)
	}
	next := append(append([]string{}, cfg.PHP.FPMPorts[version]...), fmt.Sprintf("%d:%d", host, container))
	resolved, err := SetFPMPorts(version, next)
	if err != nil {
		return 0, err
	}
	// The added mapping is the last one whose container port matches; its host is
	// the resolved (possibly shifted) port.
	for i := len(resolved) - 1; i >= 0; i-- {
		if ContainerPort(resolved[i]) == container {
			return specHostPort(resolved[i]), nil
		}
	}
	return host, nil
}

// RemoveFPMPort drops the mapping with the given host port from a version's FPM
// ports and re-applies the remainder.
func RemoveFPMPort(version string, host int) error {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return fmt.Errorf("loading global config: %w", err)
	}
	var kept []string
	for _, s := range cfg.PHP.FPMPorts[version] {
		if specHostPort(s) != host {
			kept = append(kept, s)
		}
	}
	_, err = SetFPMPorts(version, kept)
	return err
}

// parseFPMPortSpec validates a "host:container" (or "ip:host:container") mapping
// and returns its host and container ports. A bare host port is rejected because
// the container-internal port is required to publish it.
func parseFPMPortSpec(spec string) (host, container int, err error) {
	host = PrimaryHostPort([]string{spec})
	container = ContainerPort(spec)
	if host < 1 || host > 65535 || container < 1 || container > 65535 {
		return 0, 0, fmt.Errorf("invalid port mapping %q: expected host:container with each port 1-65535", spec)
	}
	return host, container, nil
}

// specHostPort returns the host-side port of a "host:container" or
// "ip:host:container" mapping (an optional /proto suffix is ignored), or 0. It
// reuses PrimaryHostPort so there is a single host-port parser for FPM specs.
func specHostPort(spec string) int {
	return PrimaryHostPort([]string{spec})
}
