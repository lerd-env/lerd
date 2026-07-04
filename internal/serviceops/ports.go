package serviceops

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// ErrPortInUse is returned by SetPublishedPort when the requested host port is
// not bindable. Callers can errors.Is on it to add a surface-specific hint (the
// CLI appends the "check which process owns it" command).
var ErrPortInUse = errors.New("port already in use")

// ErrPortReserved is returned when a requested host port is already claimed by
// another lerd service (its default, published, or extra ports). A plain
// bindability test misses this while that sibling is stopped, so the two units
// would collide at boot — this rejects the clash up front.
var ErrPortReserved = errors.New("port already claimed by another lerd service")

// PortChange reports the outcome of SetPublishedPort so each surface (CLI, MCP,
// Web UI) renders its own message from one shared code path.
type PortChange struct {
	Requested int  // the port the caller asked for (0 = reset to default)
	Actual    int  // the resulting published port (the guard may shift a reset)
	Installed bool // false when the service isn't installed (override saved only)
	WasActive bool // whether the unit was restarted to apply the change
	NoOp      bool // the requested port already matched the current override
}

// podman/quadlet seams for SetPublishedPort, swapped in tests so the
// stop/rewrite/start sequence — and its rollback when the start fails — can be
// exercised without a live container runtime.
var (
	portsUnitStatus = podman.UnitStatus
	portsStopUnit   = podman.StopUnit
	portsStartUnit  = podman.StartUnit
	portsWaitReady  = podman.WaitReady
	portsRerender   = rerenderServiceQuadlet
)

// SetPublishedPort moves a service's published host port (port > 0) or resets it
// to the preset default (port == 0), persisting the override and re-rendering and
// restarting the unit as needed. It is the single entry point shared by the CLI
// `service port` command, the MCP service:port action, and the Web UI ports
// endpoint, so all three enforce identical validation, the port-ownership guard,
// and the host-proxy follower refresh. The container-internal port is untouched.
func SetPublishedPort(name string, port int) (PortChange, error) {
	res := PortChange{Requested: port, Actual: port}
	if !config.IsDefaultPreset(name) && !ServiceInstalled(name) {
		return res, fmt.Errorf("%q is not a built-in or installed service", name)
	}
	if port < 0 || port > 65535 {
		return res, fmt.Errorf("invalid port %d: must be 0-65535", port)
	}
	// Silence the guard's shift hook during our own quadlet write; we fire it
	// once at the end with the actual resulting port, so a --reset that the guard
	// re-shifts off a host-owned default never refreshes followers twice.
	savedHook := OnPublishedPortShift
	OnPublishedPortShift = nil
	defer func() { OnPublishedPortShift = savedHook }()

	cfg, err := config.LoadGlobal()
	if err != nil {
		return res, err
	}
	svcCfg := cfg.Services[name]
	prevPublished := svcCfg.PublishedPort
	// Requesting the preset default is the same as resetting to it: normalise to 0
	// so we don't store a redundant override and, crucially, skip the bind probe
	// below — a running service holds its own default port, so probing it would
	// otherwise reject `lerd service port mysql 3306` while mysql legitimately owns
	// 3306 (the Web UI already converts the default to null client-side).
	if port > 0 && port == svcCfg.Port {
		port = 0
		res.Requested = 0
	}
	if port == svcCfg.PublishedPort {
		res.NoOp = true
		res.Actual = svcCfg.PublishedPort
		return res, nil
	}
	// A published port can't double as one of this service's own extra ports.
	if port > 0 {
		for _, ep := range svcCfg.ExtraPorts {
			if extraHostPort(ep) == port {
				return res, fmt.Errorf("%w: %d", ErrPortInUse, port)
			}
		}
	}
	// Reject a port another lerd service already claims (even a stopped one), so
	// the two units can't collide at boot.
	if port > 0 && portReservedByOther(name, port) {
		return res, fmt.Errorf("%w: %d", ErrPortReserved, port)
	}
	// Pre-flight on both loopback stacks so the restart can't fail to bind and
	// leave the service down. Uses the guard's own bindability test, not a dial,
	// so the surface and the guard agree on what "free" means.
	if port > 0 && !PortAvailable(port) {
		return res, fmt.Errorf("%w: %d", ErrPortInUse, port)
	}
	svcCfg.PublishedPort = port
	cfg.Services[name] = svcCfg
	if err := config.SaveGlobal(cfg); err != nil {
		return res, err
	}
	// Only (re)write the quadlet for an INSTALLED service; never resurrect a
	// removed unit (which would auto-start on boot and grab a host-owned port).
	// The override is saved above either way, so the next install picks it up.
	if !ServiceInstalled(name) {
		res.Installed = false
		res.Actual = port
		return res, nil
	}
	res.Installed = true
	res.Actual = port
	// The guard inside the write may override the request, so read back the actual
	// resulting primary published port, not the requested one.
	err = applyServicePortRestart(name, &res, prevPublished,
		func() int {
			actual := port
			if cfg2, lerr := config.LoadGlobal(); lerr == nil && cfg2 != nil {
				if sc, ok := cfg2.Services[name]; ok {
					actual = sc.PublishedPort
				}
			}
			return actual
		},
		func() error { return persistPublishedPort(name, prevPublished) },
	)
	if err != nil {
		return res, err
	}
	// Host-proxy sites reach the service over the published loopback port, so
	// refresh their .env to follow the change (no-op when none use it).
	if savedHook != nil {
		savedHook(name, res.Actual)
	}
	return res, nil
}

// applyServicePortRestart applies a just-persisted published-port change to a
// running service and recovers if it can't take. It stops the unit first (a live
// listener would look like a foreign owner and suppress a needed guard shift),
// re-renders the quadlet, reads the actual resulting port via rereadActual into
// res.Actual, then restarts. If the unit can't bind the new port, persistPrev
// restores the previous port in config and the unit is re-rendered and brought
// back up on it, so a failed move never leaves the service down. Shared by the
// primary and secondary port paths so both recover identically.
func applyServicePortRestart(name string, res *PortChange, prevActual int, rereadActual func() int, persistPrev func() error) error {
	unit := "lerd-" + name
	status, _ := portsUnitStatus(unit)
	res.WasActive = status == "active" || status == "activating"
	if res.WasActive {
		if err := portsStopUnit(unit); err != nil {
			return fmt.Errorf("stopping %s: %w", unit, err)
		}
	}
	if err := portsRerender(name); err != nil {
		return err
	}
	res.Actual = rereadActual()
	if !res.WasActive {
		return nil
	}
	if startErr := portsStartUnit(unit); startErr != nil {
		res.Actual = prevActual
		restored := persistPrev() == nil &&
			portsRerender(name) == nil &&
			portsStartUnit(unit) == nil
		if restored {
			_ = portsWaitReady(name, 30*time.Second)
			return fmt.Errorf("could not start %s on the new port, restored the previous port %d: %w", unit, prevActual, startErr)
		}
		return fmt.Errorf("could not start %s and could not restore the previous port, run `lerd start`: %w", unit, startErr)
	}
	_ = portsWaitReady(name, 30*time.Second)
	return nil
}

// serviceDefaultPorts returns a service's default host:container mappings — the
// preset's for a bundled service, the YAML's for an installed custom one — the
// canonical list SetPublishedPortFor resolves a container port against.
func serviceDefaultPorts(name string) []string {
	if p := config.PresetPorts(name); len(p) > 0 {
		return p
	}
	if svc, err := config.LoadCustomService(name); err == nil {
		return svc.Ports
	}
	return nil
}

// SetPublishedPortFor moves the published host port of the mapping whose
// container-internal port is containerPort, the multi-port generalisation of
// SetPublishedPort. hostPort 0 (or the mapping's preset default) clears the
// override. When containerPort is the primary mapping it delegates to
// SetPublishedPort so the primary keeps its guard and host-proxy-follower
// handling; secondary ports (mailpit's 8025 UI, rustfs' 9001 console) persist to
// PublishedPorts and re-render like an extra-ports change.
func SetPublishedPortFor(name string, containerPort, hostPort int) (PortChange, error) {
	res := PortChange{Requested: hostPort, Actual: hostPort}
	if !config.IsDefaultPreset(name) && !ServiceInstalled(name) {
		return res, fmt.Errorf("%q is not a built-in or installed service", name)
	}
	if hostPort < 0 || hostPort > 65535 {
		return res, fmt.Errorf("invalid port %d: must be 0-65535", hostPort)
	}
	ports := serviceDefaultPorts(name)
	defHost, isPrimary, found := 0, false, false
	for i, spec := range ports {
		if podman.ContainerPort(spec) == containerPort {
			defHost, isPrimary, found = podman.PrimaryHostPort([]string{spec}), i == 0, true
			break
		}
	}
	if !found {
		return res, fmt.Errorf("%s has no published port mapping for container port %d", name, containerPort)
	}
	if isPrimary {
		return SetPublishedPort(name, hostPort)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return res, err
	}
	svcCfg := cfg.Services[name]
	prevPublished := svcCfg.PublishedPorts[containerPort]
	// Requesting the preset default is a reset: normalise to 0 so we clear the
	// override rather than storing a redundant one and needlessly probing a port
	// this mapping already legitimately owns.
	if hostPort == defHost {
		hostPort = 0
		res.Requested = 0
	}
	if hostPort == prevPublished {
		res.NoOp = true
		res.Actual = prevPublished
		return res, nil
	}
	if hostPort > 0 {
		for i, spec := range ports {
			if podman.ContainerPort(spec) == containerPort {
				continue
			}
			if svcCfg.HostPortFor(podman.ContainerPort(spec), podman.PrimaryHostPort([]string{spec}), i == 0) == hostPort {
				return res, fmt.Errorf("%w: %d", ErrPortInUse, hostPort)
			}
		}
		for _, ep := range svcCfg.ExtraPorts {
			if extraHostPort(ep) == hostPort {
				return res, fmt.Errorf("%w: %d", ErrPortInUse, hostPort)
			}
		}
		if portReservedByOther(name, hostPort) {
			return res, fmt.Errorf("%w: %d", ErrPortReserved, hostPort)
		}
		if !PortAvailable(hostPort) {
			return res, fmt.Errorf("%w: %d", ErrPortInUse, hostPort)
		}
	}
	if err := persistSecondaryOverride(name, containerPort, hostPort); err != nil {
		return res, err
	}
	res.Actual = hostPort
	if !ServiceInstalled(name) {
		res.Installed = false
		return res, nil
	}
	res.Installed = true
	// Same stop-before-write and start-failure rollback the primary path gets: a
	// secondary move whose new port can't bind must not leave the unit down.
	return res, applyServicePortRestart(name, &res, prevPublished,
		func() int {
			actual := hostPort
			if cfg2, lerr := config.LoadGlobal(); lerr == nil && cfg2 != nil {
				if sc, ok := cfg2.Services[name]; ok {
					actual = sc.PublishedPorts[containerPort]
				}
			}
			return actual
		},
		func() error { return persistSecondaryOverride(name, containerPort, prevPublished) },
	)
}

// persistSecondaryOverride records port as the host override for the mapping whose
// container-internal port is containerPort, or clears the override when port is 0.
// The delete-aware form SetPublishedPortFor uses for both its write and its
// rollback, so restoring a mapping that had no prior override leaves the config
// clean rather than pinning it to a stale 0.
func persistSecondaryOverride(name string, containerPort, port int) error {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return fmt.Errorf("loading global config: %w", err)
	}
	entry := cfg.Services[name]
	if port > 0 {
		if entry.PublishedPorts == nil {
			entry.PublishedPorts = map[int]int{}
		}
		entry.PublishedPorts[containerPort] = port
	} else {
		delete(entry.PublishedPorts, containerPort)
		if len(entry.PublishedPorts) == 0 {
			entry.PublishedPorts = nil
		}
	}
	cfg.Services[name] = entry
	return config.SaveGlobal(cfg)
}

// SetExtraPorts replaces a bundled preset's extra published ports with ports
// (each a bare "host", "host:container", or "ip:host:container" mapping),
// de-duplicating and validating, then re-rendering and restarting the unit when
// it is running. Shared by the CLI `service expose`, MCP service:expose, and the
// Web UI ports endpoint. Any preset lerd ships qualifies (default-stack or
// optional like gotenberg); genuinely custom services declare their ports in
// their own YAML, so they're excluded.
func SetExtraPorts(name string, ports []string) error {
	if !config.PresetExists(name) {
		return fmt.Errorf("%q is not a bundled service", name)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	svcCfg := cfg.Services[name]
	// The service's own main host port (published override, else preset default)
	// is reserved — an extra mapping must not republish it.
	mainHost := svcCfg.Port
	if svcCfg.PublishedPort > 0 {
		mainHost = svcCfg.PublishedPort
	}
	clean := make([]string, 0, len(ports))
	seenSpec := map[string]bool{}
	seenHost := map[int]bool{}
	for _, p := range ports {
		p = strings.TrimSpace(p)
		if p == "" || seenSpec[p] {
			continue
		}
		if err := ValidateExtraPort(p); err != nil {
			return err
		}
		host := extraHostPort(p)
		if host > 0 {
			if host == mainHost || seenHost[host] {
				return fmt.Errorf("%w: %d", ErrPortInUse, host)
			}
			if portReservedByOther(name, host) {
				return fmt.Errorf("%w: %d", ErrPortReserved, host)
			}
			seenHost[host] = true
		}
		seenSpec[p] = true
		clean = append(clean, p)
	}
	svcCfg.ExtraPorts = clean
	cfg.Services[name] = svcCfg
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}
	if !ServiceInstalled(name) {
		return nil
	}
	if err := rerenderServiceQuadlet(name); err != nil {
		return err
	}
	if status, _ := podman.UnitStatus("lerd-" + name); status == "active" {
		_ = podman.RestartUnit("lerd-" + name)
	}
	return nil
}

// AddExtraPort adds a single extra published port to a built-in service. Adding a
// mapping already present is a harmless re-render (SetExtraPorts de-duplicates).
func AddExtraPort(name, spec string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	cur := cfg.Services[name].ExtraPorts
	return SetExtraPorts(name, append(append([]string{}, cur...), spec))
}

// RemoveExtraPort removes a single extra published port from a built-in service.
func RemoveExtraPort(name, spec string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	return SetExtraPorts(name, removePort(append([]string{}, cfg.Services[name].ExtraPorts...), spec))
}

// ValidateExtraPort checks that spec is a usable podman port mapping: a bare host
// port, "host:container", or "ip:host:container", each port in 0-65535, with an
// optional "/tcp" or "/udp" suffix.
func ValidateExtraPort(spec string) error {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return fmt.Errorf("empty port mapping")
	}
	body := strings.SplitN(spec, "/", 2)[0]
	parts := strings.Split(body, ":")
	if len(parts) == 0 || len(parts) > 3 {
		return fmt.Errorf("invalid port mapping %q", spec)
	}
	nums := parts
	if len(parts) == 3 {
		nums = parts[1:] // ip:host:container — the IP isn't a port
	}
	for _, n := range nums {
		v, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil || v < 0 || v > 65535 {
			return fmt.Errorf("invalid port %q in mapping %q: must be 0-65535", strings.TrimSpace(n), spec)
		}
	}
	return nil
}

// rerenderServiceQuadlet rewrites a service's unit file from its current config,
// dispatching on whether it's a built-in preset or an installed custom service.
func rerenderServiceQuadlet(name string) error {
	if config.IsDefaultPreset(name) {
		return EnsureDefaultPresetQuadlet(name)
	}
	svc, err := config.LoadCustomService(name)
	if err != nil {
		return fmt.Errorf("loading custom service %q: %w", name, err)
	}
	return EnsureCustomServiceQuadlet(svc)
}

// portReservedByOther reports whether host port p is already claimed by a lerd
// service other than self: its effective primary, its extra ports, and — unlike a
// bare HostPorts() read — a multi-port service's un-overridden SECONDARY default
// ports too, so a stopped mailpit still reserves its 8025 web UI.
func portReservedByOther(self string, p int) bool {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return false
	}
	for svcName, svc := range cfg.Services {
		if svcName == self {
			continue
		}
		for _, hp := range serviceEffectiveHostPorts(svcName, svc) {
			if hp == p {
				return true
			}
		}
	}
	return false
}

// serviceEffectiveHostPorts returns every host port a service will actually
// publish: each default mapping resolved through its override (primary or per-port
// via HostPortFor), plus its extra ports. It differs from ServiceConfig.HostPorts
// by also reporting a multi-port service's un-overridden secondary default ports
// (mailpit's 8025 UI, rustfs' 9001 console), which no config field records, so the
// interactive reservation check can't hand another service a port a stopped
// sibling owns by default and collide at boot.
func serviceEffectiveHostPorts(name string, cfg config.ServiceConfig) []int {
	seen := map[int]bool{}
	var ports []int
	add := func(n int) {
		if n > 0 && !seen[n] {
			seen[n] = true
			ports = append(ports, n)
		}
	}
	for i, spec := range serviceDefaultPorts(name) {
		add(cfg.HostPortFor(podman.ContainerPort(spec), podman.PrimaryHostPort([]string{spec}), i == 0))
	}
	for _, hp := range cfg.HostPorts() {
		add(hp)
	}
	return ports
}

// extraHostPort returns the host-side port of a "host", "host:container", or
// "ip:host:container" mapping (an optional /proto suffix is ignored), or 0 when
// none is parseable.
func extraHostPort(spec string) int {
	parts := strings.Split(strings.SplitN(spec, "/", 2)[0], ":")
	host := parts[0]
	if len(parts) > 1 {
		host = parts[len(parts)-2]
	}
	n, _ := strconv.Atoi(strings.TrimSpace(host))
	return n
}

func removePort(ports []string, port string) []string {
	out := ports[:0]
	for _, p := range ports {
		if p != port {
			out = append(out, p)
		}
	}
	return out
}

// PublishedPortSnapshot captures a service's published-port configuration so a
// multi-step change (the Web UI ports modal applies the primary port, the
// secondary ports, and the extra ports in sequence) can be rolled back to a
// consistent pre-save state when a later step fails.
type PublishedPortSnapshot struct {
	PublishedPort  int
	PublishedPorts map[int]int
	ExtraPorts     []string
}

// SnapshotPublishedPorts records a service's current published-port config. The
// bool is false only when the global config can't be read, in which case the
// caller should skip the transactional rollback rather than restore junk.
func SnapshotPublishedPorts(name string) (PublishedPortSnapshot, bool) {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return PublishedPortSnapshot{}, false
	}
	sc := cfg.Services[name]
	snap := PublishedPortSnapshot{PublishedPort: sc.PublishedPort}
	if len(sc.PublishedPorts) > 0 {
		snap.PublishedPorts = make(map[int]int, len(sc.PublishedPorts))
		for k, v := range sc.PublishedPorts {
			snap.PublishedPorts[k] = v
		}
	}
	if len(sc.ExtraPorts) > 0 {
		snap.ExtraPorts = append([]string{}, sc.ExtraPorts...)
	}
	return snap, true
}

// RestorePublishedPorts rolls a service's published-port config back to a snapshot
// and re-renders and restarts the unit so the running service matches, undoing a
// partially applied ports-modal save. It restores the whole port set at once, so a
// mapping the failed save had already moved returns to its prior port.
func RestorePublishedPorts(name string, snap PublishedPortSnapshot) error {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return fmt.Errorf("loading global config: %w", err)
	}
	entry := cfg.Services[name]
	entry.PublishedPort = snap.PublishedPort
	entry.PublishedPorts = snap.PublishedPorts
	entry.ExtraPorts = snap.ExtraPorts
	cfg.Services[name] = entry
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}
	if !ServiceInstalled(name) {
		return nil
	}
	unit := "lerd-" + name
	wasActive := unitActive(name)
	if wasActive {
		_ = portsStopUnit(unit)
	}
	if err := portsRerender(name); err != nil {
		return err
	}
	if wasActive {
		if err := portsStartUnit(unit); err != nil {
			return err
		}
		_ = portsWaitReady(name, 30*time.Second)
	}
	return nil
}
