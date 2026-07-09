package watcher

import (
	"net"
	"time"

	"github.com/geodro/lerd/internal/podman"
)

// hostGatewayDeps is the injection surface for tickHostGateway so the
// orchestration can be unit-tested without spinning up lerd-nginx.
type hostGatewayDeps struct {
	primaryLANIP       func() string
	readCurrent        func() string
	reachable          func(ip string) bool
	detectFresh        func() string
	readNginxIP        func() string
	readBrowserNginxIP func() string
	freshNginxIP       func() string
	writeHosts         func(hostIP, nginxIP string) error
	onUpdate           func()
	log                func(level, msg string, kv ...any)
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

// hostGatewayDepsFromPodman wires the tick to the real podman package and the
// real hosts files. Split out so an integration test can drive the genuine
// read, inspect and write path against a fake podman on PATH.
func hostGatewayDepsFromPodman() hostGatewayDeps {
	return hostGatewayDeps{
		primaryLANIP:       primaryLANIP,
		readCurrent:        podman.ReadHostGatewayFromFile,
		reachable:          podman.HostReachable,
		detectFresh:        podman.DetectHostGatewayIPProbeOnly,
		readNginxIP:        podman.ReadNginxIPFromFile,
		readBrowserNginxIP: podman.ReadBrowserNginxIPFromFile,
		freshNginxIP:       podman.LookupNginxContainerIP,
		writeHosts:         podman.WriteContainerHostsWith,
		onUpdate:           OnGatewayIPChange,
		log: func(level, msg string, kv ...any) {
			switch level {
			case "info":
				logger.Info(msg, kv...)
			case "warn":
				logger.Warn(msg, kv...)
			}
		},
	}
}

// WatchHostGateway keeps both addresses in the shared hosts files fresh:
// lerd-nginx's bridge IP, and the host.containers.internal gateway that Xdebug
// needs. Costs one podman inspect per tick; the exec probe stays LAN-gated.
// Runs until stop is closed; pass nil to run forever.
func WatchHostGateway(interval time.Duration, stop <-chan struct{}) {
	deps := hostGatewayDepsFromPodman()
	state := &hostGatewayState{lastLAN: primaryLANIP()}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			tickHostGateway(deps, state)
		case <-stop:
			return
		}
	}
}

// tickHostGateway runs one iteration of the watch loop, checking the two
// addresses baked into the hosts files. The nginx IP is looked up once and
// threaded into both halves, so a tick costs exactly one podman inspect.
func tickHostGateway(d hostGatewayDeps, s *hostGatewayState) {
	nginxIP := d.freshNginxIP()
	tickNginxIP(d, nginxIP)
	tickGatewayIP(d, s, nginxIP)
}

// tickNginxIP repoints the hosts files when lerd-nginx returns on a new bridge
// IP, as it does on every recreation. It rewrites when either file has drifted,
// so a write that updated one and failed on the other is retried next tick.
func tickNginxIP(d hostGatewayDeps, fresh string) {
	if fresh == "" {
		return
	}
	current := d.readNginxIP()
	if current == "" {
		return
	}
	if current == fresh && d.readBrowserNginxIP() == fresh {
		return
	}
	// Pass the gateway back in untouched. Re-detecting it here would let an
	// nginx-only rewrite move host.containers.internal without firing
	// onUpdate, stranding the host-proxy vhosts on the old address.
	gateway := d.readCurrent()
	if gateway == "" {
		return
	}
	if err := d.writeHosts(gateway, fresh); err != nil {
		d.log("warn", "rewriting container hosts file", "err", err)
		return
	}
	d.log("info", "container hosts files repointed at lerd-nginx", "ip", fresh)
}

// tickGatewayIP keeps host.containers.internal pointing at a routable address.
// The fast path (LAN IP unchanged since last tick) returns without a podman
// exec; the slow path fires only when the laptop actually moved networks.
func tickGatewayIP(d hostGatewayDeps, s *hostGatewayState, nginxIP string) {
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
	if err := d.writeHosts(fresh, nginxIPForWrite(d, nginxIP)); err != nil {
		d.log("warn", "rewriting container hosts file", "err", err)
		return
	}
	d.log("info", "host gateway IP updated", "old", current, "new", fresh)
	if d.onUpdate != nil {
		d.onUpdate()
	}
}

// nginxIPForWrite prefers the live address, keeps whatever the file already
// pins when nginx is down, and only then falls back to loopback so the site
// entries stay well-formed.
func nginxIPForWrite(d hostGatewayDeps, live string) string {
	if live != "" {
		return live
	}
	if onDisk := d.readNginxIP(); onDisk != "" {
		return onDisk
	}
	return "127.0.0.1"
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
