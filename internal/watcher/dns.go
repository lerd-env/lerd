package watcher

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/eventbus"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/systemd"
)

// idleSkipEveryN controls how aggressively to back off polling when the
// session is idle or locked. We still probe once every N ticks so a
// returning user hits a healed DNS immediately.
const idleSkipEveryN = 10

// dnsWatchDeps is the injection surface for tickDNS so the orchestration
// can be unit-tested without an actual resolver or eventbus subscriber.
type dnsWatchDeps struct {
	check               func(tld string) (bool, error)
	waitReady           func(time.Duration) error
	configureResolver   func() error
	repairPossible      func() bool
	idleOrLocked        func() bool
	publishStatus       func()
	dnsEnvFingerprint   func() string
	resyncContainerDNS  func() error
	repairExposeMapping func() (changed bool, err error)
	// nginxHealthy reports whether lerd-nginx is up and accepting on 443;
	// repairNginx re-establishes it. A host resume can stop lerd-nginx or break
	// its rootless port-forward, so .test sites return "Secure Connection Failed"
	// until a manual lerd restart. Both nil off the Linux rootless-podman path.
	nginxHealthy func() bool
	repairNginx  func() error
	// isStopped reports an intentional `lerd stop`, after which the watcher must
	// not restart anything the user stopped on purpose. now is the clock (a seam
	// for tests) used to detect a resume from the wall-clock gap between ticks.
	// All nil off Linux / in tests that don't exercise them.
	isStopped func() bool
	now       func() time.Time
	log       func(level, msg string, kv ...any)
}

// dnsWatchState is the cross-tick memory for WatchDNS. lastOK starts nil
// so the first observation always publishes, in case the snapshot built
// during boot baked in a stale dns.ok=false.
type dnsWatchState struct {
	lastOK            *bool
	tickCount         int
	repairUnavailable bool
	dnsEnv            string
	dnsEnvSeen        bool
	lastTick          time.Time
}

// resumeGapThreshold: a wall-clock gap larger than this between two consecutive
// watcher ticks means the host was suspended. The monotonic ticker is frozen
// during suspend while the wall clock keeps advancing, so a large wall gap with
// no intervening ticks is an unambiguous "just resumed" signal — unlike a `lerd
// start`, which a fresh watcher never mistakes for a resume (its first tick has
// no previous timestamp to compare against).
const resumeGapThreshold = 90 * time.Second

// nginxHealthyDialTimeout bounds the 443 connectivity probe.
const nginxHealthyDialTimeout = time.Second

// resumed records this tick's time and reports whether the host just woke from
// suspend, measured as the wall-clock gap since the previous tick. Comparing the
// monotonic-stripped times (Round(0)) is what reveals the suspend, since the
// monotonic delta the ticker runs on does not advance while suspended.
func (s *dnsWatchState) resumed(now time.Time) bool {
	prev := s.lastTick
	s.lastTick = now
	if prev.IsZero() {
		return false
	}
	return now.Round(0).Sub(prev.Round(0)) > resumeGapThreshold
}

// defaultDNSEnvFingerprint summarises the host DNS environment: the sorted
// upstream resolver set plus whether a VPN tunnel is up. Either changing
// (VPN connect/disconnect, network switch) means aardvark-dns is serving
// stale forwarders and a stale cache, so container DNS needs a re-sync.
func defaultDNSEnvFingerprint() string {
	up := dns.ReadUpstreamDNS()
	sort.Strings(up)
	vpn := "0"
	if dns.VPNActive() {
		vpn = "1"
	}
	return strings.Join(up, ",") + "|" + vpn
}

// defaultResyncContainerDNS re-points the lerd network's aardvark-dns at
// the current host resolvers and reloads the network so containers pick
// them up. This is the automatic equivalent of a manual `lerd restart`
// after a VPN connects.
func defaultResyncContainerDNS() error {
	if err := podman.EnsureNetworkDNS("lerd", dns.ReadContainerDNS()); err != nil {
		return err
	}
	return podman.ReloadNetworks()
}

// defaultNginxHealthy reports whether lerd-nginx is serving on its configured
// HTTPS port by a single loopback dial. A failed connect is the signal — a resume
// drops the listener entirely (stopped container or dead forward). It does not
// also inspect the container, since ContainerRunning swallows a transient `podman
// inspect` error (common right after a resume) as "not running" and would bounce
// a healthy nginx.
func defaultNginxHealthy() bool {
	port := 443
	if cfg, err := config.LoadGlobal(); err == nil && cfg != nil && cfg.Nginx.HTTPSPort > 0 {
		port = cfg.Nginx.HTTPSPort
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), nginxHealthyDialTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// defaultRepairNginx restarts nginx so it rebinds its host ports on the live
// network. It deliberately stays this minimal: the network itself is the watcher's
// DNS re-sync path's job or, for a dual-stack migration, `lerd start`'s (recreating
// the network tears down every container, far too destructive for a background
// timer), and it does NOT touch the vhost registry, since downgrading a Secured
// site to HTTP over a cert mount that hasn't returned yet would silently drop its
// HTTPS. If nginx can't rebind (e.g. that pending migration), the restart is a
// no-op the next probe still sees as down, surfacing it for `lerd start`.
func defaultRepairNginx() error {
	return podman.RestartUnit("lerd-nginx")
}

// publishOnTransition sets *cur to next and fires pub when the value changed (or
// on the first observation, when *cur is nil). The shared transition-and-publish
// step for the nginx and DNS health states.
func publishOnTransition(cur **bool, next bool, pub func()) {
	if *cur == nil || **cur != next {
		v := next
		*cur = &v
		pub()
	}
}

// healNginxOnResume restarts nginx if it isn't serving, but only on the tick that
// just detected a resume. A resume is a discrete, unambiguous trigger, so there is
// no polling, debounce, or seen-healthy guard, and no way to race a concurrent
// `lerd start` (a start doesn't suspend the machine). systemd's Restart=always
// already covers an nginx crash; this covers the resume case it can't see, where
// the container stays "running" but its rootless 443 forward is dead. It honors an
// intentional `lerd stop` and is a no-op when nginxHealthy is unset (non-Linux).
//
// If the watcher isn't running at the moment of resume (e.g. it restarted during
// the outage), that resume is missed and the user falls back to `lerd start`; a
// rare edge, far better than a continuous poll that fights every bring-up.
func healNginxOnResume(d dnsWatchDeps) {
	if d.nginxHealthy == nil {
		return
	}
	if d.isStopped != nil && d.isStopped() {
		return
	}
	if d.nginxHealthy() {
		return // the resume didn't break it
	}
	d.log("warn", "nginx not serving after resume, restarting")
	if d.repairNginx == nil {
		return
	}
	if err := d.repairNginx(); err != nil {
		d.log("error", "nginx restart after resume failed", "err", err)
	}
}

// defaultRepairExposeMapping re-renders the host dnsmasq .tld answer to the
// current primary LAN IP and reloads lerd-dns when lan:expose is on and the
// published mapping has drifted. lerd only regenerates that mapping on
// `lerd start`, so a sleep/wake DHCP renew or a network switch leaves dnsmasq
// answering the old IP; CheckStatus compares that answer against the live
// primaryLANIP and reports the dashboard pill down even though lerd-dns is
// serving fine, and in lan:expose mode the published address eventually stops
// routing once the old lease is gone. The config dir (DnsmasqDir) is
// user-owned and mounted read-only into the lerd-dns container, so the
// rewrite needs no privilege escalation and a unit reload picks it up on both
// macOS (launchd) and Linux (systemd).
//
// Returns (false, nil), a safe no-op, when expose is off, the host has no
// LAN IP yet, or the mapping already matches, so it can run on every failed
// health tick without thrashing the daemon.
func defaultRepairExposeMapping(tld string) (bool, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return false, err
	}
	if cfg == nil || !cfg.LAN.Exposed {
		return false, nil
	}
	if primaryLANIP() == "" {
		return false, nil
	}
	// Gate the restart on the rendered config actually changing, not on the live
	// dnsmasq answer: a freshly restarted lerd-dns lags a tick or two before it
	// serves the new IP, and keying off the answer would re-render and restart it
	// again every failed tick until it settled. Comparing the on-disk config also
	// covers the AAAA record (DnsmasqAnswer is IPv4-only, so an answer check would
	// never heal an IPv6 drift), and makes the repair idempotent so it restarts
	// exactly once per real drift.
	confPath := filepath.Join(config.DnsmasqDir(), "lerd.conf")
	before, _ := os.ReadFile(confPath)
	if err := dns.WriteDnsmasqConfig(config.DnsmasqDir()); err != nil {
		return false, err
	}
	after, _ := os.ReadFile(confPath)
	if !exposeConfigChanged(before, after) {
		return false, nil
	}
	return true, podman.RestartUnit("lerd-dns")
}

// exposeConfigChanged reports whether the rendered dnsmasq config changed in a
// way that warrants restarting lerd-dns. A change confined to the AAAA (IPv6)
// address lines is ignored: a global v6 coming and going, or a privacy address
// rotating, would otherwise restart lerd-dns on every tick, while v4 is what
// LAN clients overwhelmingly rely on. The new config is still written to disk,
// so a later v4 drift or a fresh setup picks up the current AAAA.
func exposeConfigChanged(before, after []byte) bool {
	return !bytes.Equal(stripAAAALines(before), stripAAAALines(after))
}

// stripAAAALines drops the IPv6 `address=/.test/<v6>` lines (those whose target
// contains a colon) so AAAA-only drift compares equal.
func stripAAAALines(conf []byte) []byte {
	lines := strings.Split(string(conf), "\n")
	kept := lines[:0]
	for _, ln := range lines {
		if strings.HasPrefix(ln, "address=/") && strings.Contains(ln[strings.LastIndex(ln, "/")+1:], ":") {
			continue
		}
		kept = append(kept, ln)
	}
	return []byte(strings.Join(kept, "\n"))
}

// linkChangeDebounce caps how long the netlink burst from a single VPN
// connect or disconnect is allowed to settle before we re-tick. The kernel
// emits a flurry of RTM_NEWLINK / RTM_NEWADDR over the first few hundred
// milliseconds; re-syncing mid-burst would aim aardvark-dns at an
// intermediate resolver set that the network doesn't actually settle on.
const linkChangeDebounce = 750 * time.Millisecond

// WatchDNS polls DNS health for the given TLD every interval. When resolution
// is broken it waits for lerd-dns to be ready and re-applies the resolver
// configuration, replicating the DNS repair done by lerd start. When the
// user session is idle or locked it backs off to one probe every 10 ticks
// so laptops don't pay the per-30s DNS lookup battery cost while away.
//
// On Linux it also subscribes to rtnetlink link and address changes via
// linkChanges, so a VPN connect or disconnect kicks an immediate tick
// instead of waiting up to interval for the next poll. The poll stays as
// a safety net so a missed netlink event can't strand the system.
//
// Every observed transition and every successful repair publishes
// eventbus.KindStatus so the dashboard reflects the live state via the
// WebSocket without a manual refresh.
func WatchDNS(interval time.Duration, tld string) {
	deps := dnsWatchDeps{
		check:             dns.Check,
		waitReady:         dns.WaitReady,
		configureResolver: dns.ConfigureResolver,
		repairPossible:    dns.RepairPossible,
		idleOrLocked:      systemd.SessionIsIdleOrLocked,
		publishStatus:     func() { eventbus.Default.Publish(eventbus.KindStatus) },
		now:               time.Now,
		log: func(level, msg string, kv ...any) {
			switch level {
			case "info":
				logger.Info(msg, kv...)
			case "warn":
				logger.Warn(msg, kv...)
			case "error":
				logger.Error(msg, kv...)
			}
		},
	}

	// Container DNS re-sync recovers from aardvark-dns forwarder staleness,
	// which is specific to Linux rootless podman. macOS containers get DNS
	// from the podman machine VM (ReadContainerDNS is nil there), so there
	// is nothing to re-sync and the detection stays off.
	if runtime.GOOS == "linux" {
		deps.dnsEnvFingerprint = defaultDNSEnvFingerprint
		deps.resyncContainerDNS = defaultResyncContainerDNS
		// Restart nginx after a host resume leaves rootless networking in a bad
		// state so .test sites return "Secure Connection Failed" until a manual
		// lerd restart (issue #665). DNS resolution is already repaired below.
		deps.nginxHealthy = defaultNginxHealthy
		deps.repairNginx = defaultRepairNginx
		deps.isStopped = config.IsStopped
	}

	// Cross-platform: heal a stale lan:expose .tld mapping after the host LAN
	// IP changes. Bind the configured TLD so the tick body stays no-arg.
	deps.repairExposeMapping = func() (bool, error) { return defaultRepairExposeMapping(tld) }

	state := &dnsWatchState{}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	done := make(chan struct{})
	defer close(done)

	linkRaw := make(chan struct{}, 32)
	linkSettled := make(chan struct{}, 4)
	go superviseLinkChanges(dns.LinkChanges, linkRaw, done, linkChangeBackoff, linkChangeResetAfter)
	go dns.DebounceEvents(linkRaw, linkSettled, linkChangeDebounce, done)

	runDNSLoop(deps, state, tld, ticker.C, linkSettled, done)
}

// linkChangeBackoff is the capped exponential delay before restarting the
// host-network-change watcher after a transient error: 1s, 2s, 4s … up to 30s.
func linkChangeBackoff(attempt int) time.Duration {
	d := time.Second << attempt
	if d <= 0 || d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// linkChangeResetAfter is how long a single watch must stay up before the next
// restart is treated as fresh: a watch that ran healthy for this long and then
// hit one late transient error should restart promptly, not at the grown 30s
// cap. Passed into superviseLinkChanges so tests can shrink it without a shared
// global.
const linkChangeResetAfter = 60 * time.Second

// superviseLinkChanges keeps dns.LinkChanges alive: when it returns an error
// before done closes (a transient netlink/route socket failure) it restarts it
// after a backoff, so the watcher doesn't go permanently deaf to host network
// changes and fall back to the safety-net poll for the rest of the daemon's
// life. It returns once done is closed or LinkChanges returns nil (a clean
// shutdown, or an unsupported platform that simply waits on done).
func superviseLinkChanges(fn func(chan<- struct{}, <-chan struct{}) error,
	out chan<- struct{}, done <-chan struct{}, backoff func(int) time.Duration, resetAfter time.Duration) {

	attempt := 0
	for {
		start := time.Now()
		err := fn(out, done)
		healthyRun := time.Since(start) >= resetAfter
		select {
		case <-done:
			return
		default:
		}
		if err == nil {
			return
		}
		// A watch that stayed up a long time before failing was healthy, so one
		// late transient error shouldn't inherit the grown backoff and leave us
		// capped at 30s; restart it promptly instead.
		if healthyRun {
			attempt = 0
		}
		logger.Warn("host network change watcher errored, restarting; DNS reacts on the safety-net poll meanwhile",
			"err", err, "attempt", attempt+1)
		select {
		case <-time.After(backoff(attempt)):
		case <-done:
			return
		}
		attempt++
	}
}

// runDNSLoop runs the tick state machine: an immediate first probe, then
// a tick on every ticker fire and every settled link change. done unblocks
// the loop so tests can shut down deterministically.
func runDNSLoop(d dnsWatchDeps, state *dnsWatchState, tld string,
	tickerC <-chan time.Time, linkC <-chan struct{}, done <-chan struct{}) {

	tickDNS(d, state, tld, false)
	for {
		select {
		case <-done:
			return
		case <-tickerC:
			tickDNS(d, state, tld, false)
		case <-linkC:
			// A real host network change must re-probe immediately, even on
			// an idle/locked session, so it bypasses the polling backoff.
			tickDNS(d, state, tld, true)
		}
	}
}

// tickDNS runs one iteration of the DNS health loop. A poll tick returns
// early during idle backoff; a linkTriggered tick (a host network change)
// always probes since that is the event the watcher exists to react to. On
// every tick that probes, the previous observation is compared and a
// transition publishes KindStatus.
func tickDNS(d dnsWatchDeps, s *dnsWatchState, tld string, linkTriggered bool) {
	s.tickCount++

	// Detect a resume from the wall-clock gap since the previous tick (lastTick is
	// recorded even on the stopped/idle early returns below, so the gap stays
	// accurate). A deliberately stopped stack is left entirely alone.
	resumed := false
	if d.now != nil {
		resumed = s.resumed(d.now())
	}
	if d.isStopped != nil && d.isStopped() {
		return
	}

	// An idle/locked poll tick backs off — but never skips a detected resume,
	// since that is the moment the nginx heal exists to react to.
	if !linkTriggered && !resumed && d.idleOrLocked() && s.tickCount%idleSkipEveryN != 0 {
		return
	}

	// Re-sync container DNS when the host resolver environment changes
	// (VPN connect/disconnect, network switch). The lerd network's
	// aardvark-dns is otherwise left on the pre-change forwarders and a
	// stale cache, so containers can't resolve newly-routable hostnames
	// until a manual `lerd restart`. The first tick only records the
	// baseline so a fresh watcher start never triggers a re-sync.
	if d.dnsEnvFingerprint != nil {
		fp := d.dnsEnvFingerprint()
		if s.dnsEnvSeen && fp != s.dnsEnv {
			d.log("info", "host DNS changed, re-syncing container DNS")
			if d.resyncContainerDNS != nil {
				if err := d.resyncContainerDNS(); err != nil {
					d.log("warn", "container DNS re-sync failed", "err", err)
				}
			}
		}
		s.dnsEnv = fp
		s.dnsEnvSeen = true
	}

	// Recover nginx after a resume, once the container DNS above has re-synced the
	// network it will rebind onto.
	if resumed {
		healNginxOnResume(d)
	}

	ok, _ := d.check(tld)
	publishOnTransition(&s.lastOK, ok, d.publishStatus)
	if ok {
		return
	}

	// A stale lan:expose mapping (the dnsmasq .tld answer drifting from the
	// host's current primary LAN IP after a sleep/wake or DHCP renew) makes
	// CheckStatus report down even though lerd-dns is healthy. Re-render the
	// mapping and reload lerd-dns first: the config dir is user-owned, so this
	// needs no privilege escalation and heals even on a host where the
	// sudo-gated resolver repair below is unavailable. A no-op (expose off or
	// the mapping already current) returns false and falls through.
	if d.repairExposeMapping != nil {
		switch changed, err := d.repairExposeMapping(); {
		case err != nil:
			d.log("warn", "lan:expose DNS mapping repair failed", "err", err)
		case changed:
			d.log("info", "lan:expose DNS mapping re-rendered to the current LAN IP")
			if ok2, _ := d.check(tld); ok2 {
				up := true
				s.lastOK = &up
				d.publishStatus()
				return
			}
		}
	}

	// Skip repair when the platform can't write the resolver config from
	// this process (macOS without /etc/sudoers.d/lerd in place). Logging
	// this every tick would spam — emit once and remember the gate.
	if d.repairPossible != nil && !d.repairPossible() {
		if !s.repairUnavailable {
			d.log("warn", "DNS resolution broken; automatic repair unavailable on this host (run lerd install to grant the watcher resolver write access)", "tld", tld)
			s.repairUnavailable = true
		}
		return
	}
	s.repairUnavailable = false

	d.log("warn", "DNS resolution broken, repairing", "tld", tld)

	if err := d.waitReady(10 * time.Second); err != nil {
		d.log("error", "lerd-dns not ready", "err", err)
		return
	}

	if err := d.configureResolver(); err != nil {
		d.log("error", "DNS repair failed", "err", err)
		return
	}

	d.log("info", "DNS resolution restored", "tld", tld)
	// Repair flipped DNS from down to up; publish now so the dashboard doesn't
	// wait up to 30s for the next tick to notice.
	up := true
	s.lastOK = &up
	d.publishStatus()
}
