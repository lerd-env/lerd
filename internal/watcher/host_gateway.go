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

// hostGatewayInspectEvery is how many ticks pass between podman inspects once
// the addresses have settled.
//
// The two halves of this watch cost wildly different amounts. Comparing the LAN
// address is a UDP dial with no process behind it, so it runs every tick and a
// laptop changing networks is still noticed within one interval. Looking up
// lerd-nginx's bridge address forks podman, which at one fork per tick was the
// largest single source of this daemon's idle wakeups, measurably more than
// everything else it does combined. That address only moves when the container
// is recreated, so it is checked on the first tick and then only occasionally,
// snapping back to every tick as soon as anything does move.
const hostGatewayInspectEvery = 10

// hostGatewayDepsForWatch is the seam WatchHostGateway resolves its deps
// through, so a test can drive the loop's cadence without a live podman.
var hostGatewayDepsForWatch = hostGatewayDepsFromPodman

// WatchHostGateway keeps both addresses in the shared hosts files fresh:
// lerd-nginx's bridge IP, and the host.containers.internal gateway that Xdebug
// needs. Runs until stop is closed; pass nil to run forever.
func WatchHostGateway(interval time.Duration, stop <-chan struct{}) {
	deps := hostGatewayDepsForWatch()
	state := &hostGatewayState{lastLAN: primaryLANIP()}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	ticksToInspect := 0
	for {
		select {
		case <-ticker.C:
			inspect := ticksToInspect <= 0
			if tickHostGatewayWith(deps, state, inspect) {
				// Something moved: look again next tick instead of waiting out
				// the slow cadence.
				ticksToInspect = 0
				continue
			}
			if inspect {
				// This tick was the inspect, so only the ticks after it are
				// skipped; counting the full interval here would add one.
				ticksToInspect = hostGatewayInspectEvery - 1
			} else {
				ticksToInspect--
			}
		case <-stop:
			return
		}
	}
}

// tickHostGateway runs one full iteration, checking both addresses baked into
// the hosts files. Reports whether either was rewritten.
func tickHostGateway(d hostGatewayDeps, s *hostGatewayState) bool {
	return tickHostGatewayWith(d, s, true)
}

// tickHostGatewayWith is tickHostGateway with the expensive half optional. With
// inspect false it skips the podman lookup and runs only the LAN-gated gateway
// check, which then reads whatever nginx address the hosts file already pins.
func tickHostGatewayWith(d hostGatewayDeps, s *hostGatewayState, inspect bool) bool {
	nginxIP := ""
	changed := false
	if inspect {
		nginxIP = d.freshNginxIP()
		changed = tickNginxIP(d, nginxIP)
	}
	if tickGatewayIP(d, s, nginxIP) {
		changed = true
	}
	return changed
}

// tickNginxIP repoints the hosts files when lerd-nginx returns on a new bridge
// IP, as it does on every recreation. It rewrites when either file has drifted,
// so a write that updated one and failed on the other is retried next tick.
func tickNginxIP(d hostGatewayDeps, fresh string) bool {
	if fresh == "" {
		return false
	}
	current := d.readNginxIP()
	if current == "" {
		return false
	}
	if current == fresh && d.readBrowserNginxIP() == fresh {
		return false
	}
	// Pass the gateway back in untouched. Re-detecting it here would let an
	// nginx-only rewrite move host.containers.internal without firing
	// onUpdate, stranding the host-proxy vhosts on the old address.
	gateway := d.readCurrent()
	if gateway == "" {
		return false
	}
	if err := d.writeHosts(gateway, fresh); err != nil {
		d.log("warn", "rewriting container hosts file", "err", err)
		return false
	}
	d.log("info", "container hosts files repointed at lerd-nginx", "ip", fresh)
	return true
}

// tickGatewayIP keeps host.containers.internal pointing at a routable address.
// The fast path (LAN IP unchanged since last tick) returns without a podman
// exec; the slow path fires only when the laptop actually moved networks.
func tickGatewayIP(d hostGatewayDeps, s *hostGatewayState, nginxIP string) bool {
	lan := d.primaryLANIP()
	if lan == s.lastLAN {
		return false
	}
	s.lastLAN = lan

	current := d.readCurrent()
	if current != "" && d.reachable(current) {
		return false
	}
	fresh := d.detectFresh()
	if fresh == "" || fresh == current {
		return false
	}
	if err := d.writeHosts(fresh, nginxIPForWrite(d, nginxIP)); err != nil {
		d.log("warn", "rewriting container hosts file", "err", err)
		return false
	}
	d.log("info", "host gateway IP updated", "old", current, "new", fresh)
	if d.onUpdate != nil {
		d.onUpdate()
	}
	return true
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
