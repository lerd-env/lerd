package watcher

import (
	"net"
	"time"

	"github.com/geodro/lerd/internal/podman"
)

// hostGatewayDeps is the injection surface for tickHostGateway so the
// orchestration can be unit-tested without spinning up lerd-nginx.
type hostGatewayDeps struct {
	primaryLANIP func() string
	readCurrent  func() string
	reachable    func(ip string) bool
	detectFresh  func() string
	readNginxIP  func() string
	freshNginxIP func() string
	writeHosts   func() error
	onUpdate     func()
	log          func(level, msg string, kv ...any)
}

// OnGatewayIPChange, if set, runs after the shared hosts file is rewritten with
// a fresh host-gateway IP. The cli package wires this (via main) to regenerate
// host-proxy vhosts, which bake the gateway IP into proxy_pass on Linux and
// would otherwise point at a dead address until the next manual regen.
var OnGatewayIPChange func()

// hostGatewayState is the cross-tick memory for WatchHostGateway. We only
// keep the last-seen LAN IP so the fast path (compare and skip) is one
// cheap UDP dial with no container exec. Promoted out of the tick
// function so tests can seed it.
type hostGatewayState struct {
	lastLAN string
}

// WatchHostGateway keeps both addresses in the shared hosts files fresh:
// lerd-nginx's bridge IP, and the host.containers.internal gateway that Xdebug
// needs. Costs one podman inspect per tick; the exec probe stays LAN-gated.
func WatchHostGateway(interval time.Duration) {
	deps := hostGatewayDeps{
		primaryLANIP: primaryLANIP,
		readCurrent:  podman.ReadHostGatewayFromFile,
		reachable:    podman.HostReachable,
		detectFresh:  podman.DetectHostGatewayIPProbeOnly,
		readNginxIP:  podman.ReadNginxIPFromFile,
		freshNginxIP: podman.NginxContainerIPProbeOnly,
		writeHosts:   podman.WriteContainerHosts,
		onUpdate:     OnGatewayIPChange,
		log: func(level, msg string, kv ...any) {
			switch level {
			case "info":
				logger.Info(msg, kv...)
			case "warn":
				logger.Warn(msg, kv...)
			}
		},
	}
	state := &hostGatewayState{lastLAN: primaryLANIP()}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		tickHostGateway(deps, state)
	}
}

// tickHostGateway runs one iteration of the watch loop, checking the two
// addresses baked into the hosts files: lerd-nginx's bridge IP, and the
// host-gateway IP behind host.containers.internal.
func tickHostGateway(d hostGatewayDeps, s *hostGatewayState) {
	tickNginxIP(d)
	tickGatewayIP(d, s)
}

// tickNginxIP repoints the hosts files when lerd-nginx returns on a new bridge
// IP, as it does on every recreation. This inspects each tick: rootless
// containers aren't host-reachable, so there is no cheaper recreation signal.
func tickNginxIP(d hostGatewayDeps) {
	fresh := d.freshNginxIP()
	if fresh == "" {
		return
	}
	current := d.readNginxIP()
	if current == "" || current == fresh {
		return
	}
	if err := d.writeHosts(); err != nil {
		d.log("warn", "rewriting container hosts file", "err", err)
		return
	}
	d.log("info", "nginx container IP updated", "old", current, "new", fresh)
}

// tickGatewayIP keeps host.containers.internal pointing at a routable address.
// The fast path (LAN IP unchanged since last tick) returns without a podman
// exec; the slow path fires only when the laptop actually moved networks.
func tickGatewayIP(d hostGatewayDeps, s *hostGatewayState) {
	lan := d.primaryLANIP()
	if lan == s.lastLAN {
		return
	}
	s.lastLAN = lan

	current := d.readCurrent()
	if current != "" && d.reachable(current) {
		return
	}
	fresh := d.detectFresh()
	if fresh == "" || fresh == current {
		return
	}
	if err := d.writeHosts(); err != nil {
		d.log("warn", "rewriting container hosts file", "err", err)
		return
	}
	d.log("info", "host gateway IP updated", "old", current, "new", fresh)
	if d.onUpdate != nil {
		d.onUpdate()
	}
}

// primaryLANIP returns the local IPv4 address the kernel would use to reach
// a public destination. Duplicates internal/podman/hosts.go's helper rather
// than importing it, because we want this watcher cost to stay micro-level
// and not pay for loading the podman package's init costs on every tick.
func primaryLANIP() string {
	conn, err := net.Dial("udp4", "1.1.1.1:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
