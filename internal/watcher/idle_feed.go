package watcher

import (
	"context"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/idle"
	"github.com/geodro/lerd/internal/push"
	"github.com/geodro/lerd/internal/reqstats"
)

// activityTracker records per-site last-active times fed by the access feed and
// control socket, read by the engine and persisted to config.IdleActivityFile()
// for lerd-ui and the CLI to render. Allocated once by StartIdle.
var activityTracker *idle.Tracker

// reqAggregator holds rolling per-site request-timing windows fed by the same
// access feed. It runs regardless of idle-suspend and is persisted to
// config.RequestStatsFile() for lerd-ui to read. Allocated once by StartIdle.
var reqAggregator *reqstats.Aggregator

// reqStore is the durable SQLite record of individual requests, written from the
// access feed for the analytics view. Buffered between save ticks and flushed in
// one batch so a busy site isn't a transaction per request. Best-effort: a failed
// open leaves it nil and the aggregator snapshot still drives the live panel.
var (
	reqStore    *reqstats.Store
	reqBufMu    sync.Mutex
	reqBuf      []reqstats.Record
	lastPrune   time.Time
	reqLastSeen = map[string]time.Time{} // per-site last request time, for cold-start detection
	coldGap     time.Duration            // a gap this long or longer marks the next request a cold start
)

// reqStatsSaveInterval is how often the request-timing snapshot is flushed to
// disk for lerd-ui. Shorter than the idle tick so the panel feels live.
const reqStatsSaveInterval = 10 * time.Second

// reqStatsPruneInterval throttles how often rows past reqstats.Retention are
// pruned, which is what keeps the DB small.
const reqStatsPruneInterval = time.Hour

// slowNotifier fires a one-time push per newly-flagged slow route on each save
// tick. Allocated once by StartIdle.
var slowNotifier = newSlowRouteNotifier()

// Idle-suspend lifecycle. The control socket is always bound (the toggle/wake
// point); everything else lives in an enabled session started by enableIdle and
// torn down by disableIdle, so a disabled feature does no work at all.
var (
	idleMu       sync.Mutex
	idleCancel   context.CancelFunc // non-nil exactly while a session runs
	idleActive   atomic.Bool        // gate for control-socket activity pings
	idleStartSrc func(stop <-chan struct{}) error
)

// StartIdle wires the idle subsystem: the control socket is always bound, while
// the session (access feed, tick, source watcher) only runs when enabled.
// sourceWatcher runs until its stop channel closes.
func StartIdle(notify func(), sourceWatcher func(stop <-chan struct{}) error) {
	if notify != nil {
		idleNotifyUI = notify
	}
	idleStartSrc = sourceWatcher
	activityTracker = idle.NewTracker(resolveHostToSite)
	idleEng = newIdleEngine(activityTracker)
	reqAggregator = reqstats.New(resolveHostToSite)
	if st, err := reqstats.OpenStore(config.RequestStatsDB()); err == nil {
		reqStore = st
		// Seed the cold-start clock from the durable store so the first request
		// after a daemon restart is judged against the real last-seen time instead
		// of counting the wake as warm and skewing the route p95.
		if seen, err := st.LastSeenBySite(); err == nil {
			reqLastSeen = seen
		}
	}
	// A request after the site has been idle at least this long is a cold start;
	// tie it to the idle-suspend timeout, the point lerd already treats a site as
	// gone quiet, so a wake's inflated time is kept out of the timing view.
	coldGap = reqstats.DefaultColdGap
	if cfg, err := config.LoadGlobal(); err == nil {
		coldGap = cfg.IdleSuspendTimeout()
	}
	go runNotifier()
	startControlSocket()

	// The access feed is bound once, for the daemon's life, and fans out to both
	// request stats (always) and idle activity (only while enabled). Idle used to
	// bind it inside its session, but timing stats must run even with idle off, so
	// there is a single always-on reader here.
	startAccessFeed()
	go runReqStatsSaver()

	// Boot memory is the persisted config flag, not the ephemeral socket. When
	// off, still resume any workers a prior session left suspended (e.g. toggled
	// off while the watcher was down) so they're never stranded stopped.
	if cfg, err := config.LoadGlobal(); err == nil && cfg.IdleSuspend.Enabled {
		enableIdle()
	} else {
		go idleEng.resumeUntilClear()
	}
}

// enableIdle starts the idle session: seed activity, bind the nginx access feed,
// run the engine tick, and start the source-file watcher, all tied to one cancel
// context. Idempotent and safe to call from boot or a control "enable".
func enableIdle() {
	idleMu.Lock()
	defer idleMu.Unlock()
	if idleCancel != nil {
		return // already running
	}
	ctx, cancel := context.WithCancel(context.Background())
	idleCancel = cancel
	idleActive.Store(true)

	seedActiveSites(activityTracker)
	_ = os.MkdirAll(config.RunDir(), 0755)
	_ = activityTracker.Save(config.IdleActivityFile())

	go idleEng.run(ctx)

	// The access feed is bound once in StartIdle and gated on idleActive, so
	// enabling just flips the gate: browsing a quiet site now wakes it. The
	// source-file watcher is the only thing this session still starts.
	if idleStartSrc != nil {
		go func() { _ = idleStartSrc(ctx.Done()) }()
	}
}

// disableIdle stops the session (tick, access feed, source watcher), then resumes
// every suspended worker in the background via resumeUntilClear, which retries so
// a suspend mid-flight isn't skipped now that no later tick will catch it.
func disableIdle() {
	idleMu.Lock()
	if idleCancel != nil {
		idleActive.Store(false)
		idleCancel()
		idleCancel = nil
	}
	idleMu.Unlock()
	go idleEng.resumeUntilClear()
}

// handleAccessDatagram fans one nginx access datagram out to both consumers:
// request-timing stats always, and idle activity only while idle-suspend is
// enabled (so a disabled feature never wakes or records). A single datagram
// carries both signals in one pipe-delimited record.
func handleAccessDatagram(b []byte) {
	if reqAggregator != nil {
		if rec, ok := reqstats.ParseAccessRecord(b); ok {
			ingestAccessRecord(rec)
		}
	}
	if !idleActive.Load() {
		return
	}
	if host := idle.ParseAccessHost(b); host != "" {
		if site := activityTracker.TouchHost(host, time.Now()); site != "" {
			idleEng.OnActivity(site)
		}
	}
}

// siteForHost is the seam the request-stats fan-out resolves through; a var so a
// test can inject a resolver without a live site registry.
var siteForHost = resolveHostToSite

// ingestAccessRecord fans one parsed access record out to the durable store and
// the live aggregator. It flags a cold start (the first request after the site
// sat idle past coldGap) and keeps it out of the aggregator, whose snapshot feeds
// the slow-route notifier and the doctor: a wake's inflated time must not trip
// them, while the store still records it (marked cold, excluded from its timing).
func ingestAccessRecord(rec reqstats.AccessRecord) {
	appServed := reqstats.IsAppRequest(rec.Status, rec.URI, rec.SecondsToMillis())
	site, resolved := siteForHost(rec.Host)
	cold := false
	if appServed && resolved {
		now := time.Now()
		reqBufMu.Lock()
		last, seen := reqLastSeen[site]
		cold = reqstats.IsColdStart(last, seen, now, coldGap)
		reqLastSeen[site] = now
		if reqStore != nil {
			sr := reqstats.RecordFrom(rec, site, now)
			sr.Cold = cold
			reqBuf = append(reqBuf, sr)
		}
		reqBufMu.Unlock()
	}
	if !cold {
		reqAggregator.Record(rec)
	}
}

// runReqStatsSaver flushes the request-timing snapshot to disk on a fixed tick
// for lerd-ui to read, and fires a one-time push for any route newly flagged as
// slow. Runs for the daemon's life, independent of idle. push.Send is a no-op
// when no subscription has opted into the slow_route kind.
func runReqStatsSaver() {
	t := time.NewTicker(reqStatsSaveInterval)
	defer t.Stop()
	for range t.C {
		if reqAggregator == nil {
			continue
		}
		snap := reqAggregator.Snapshot()
		_ = reqstats.SaveSnapshot(snap, config.RequestStatsFile())
		flushReqStore()
		domainOf := siteDomainResolver()
		for _, n := range slowNotifier.notifications(snap, domainOf) {
			_ = push.Send(n)
		}
	}
}

// flushReqStore writes the buffered access records to the durable store in one
// batch and prunes rows past the retention window on a throttled cadence, so the
// store stays bounded without a delete on every tick.
func flushReqStore() {
	if reqStore == nil {
		return
	}
	reqBufMu.Lock()
	batch := reqBuf
	reqBuf = nil
	reqBufMu.Unlock()
	if len(batch) > 0 {
		_ = reqStore.Insert(batch)
	}
	if now := time.Now(); now.Sub(lastPrune) >= reqStatsPruneInterval {
		_, _ = reqStore.Prune(now.Add(-reqstats.Retention))
		lastPrune = now
	}
}

// accessFeedRetryInterval is how often startAccessFeed retries a failed bind so
// a transient boot-time failure recovers without a full daemon restart.
const accessFeedRetryInterval = 30 * time.Second

// startAccessFeed binds the always-on access-feed reader. A bind that fails at
// boot (a transient FS/permission hiccup on the socket path) is retried in the
// background so the feed and its consumers recover on their own; until it binds
// both consumers degrade to their other signals.
func startAccessFeed() {
	if conn, ok := accessFeedConn(); ok {
		go readDatagrams(conn, handleAccessDatagram)
		return
	}
	go func() {
		t := time.NewTicker(accessFeedRetryInterval)
		defer t.Stop()
		for range t.C {
			if conn, ok := accessFeedConn(); ok {
				go readDatagrams(conn, handleAccessDatagram)
				return
			}
		}
	}()
}

// accessFeedConn binds the nginx access feed listener per platform: a unix
// datagram socket on Linux (bind-mounted into the rootless nginx container), or
// UDP on macOS where nginx is in the VM and the host socket isn't reachable.
func accessFeedConn() (net.PacketConn, bool) {
	if runtime.GOOS == "darwin" {
		return listenUDP(config.AccessFeedListenAddr())
	}
	return listenDatagram(config.AccessSocketPath())
}

// listenUDP binds a UDP socket at addr for the macOS access feed. ok=false on
// failure, matching listenDatagram so callers skip the feed the same way.
func listenUDP(addr string) (net.PacketConn, bool) {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, false
	}
	return conn, true
}

// listenDatagram binds a unix datagram socket at path under RunDir, replacing any
// stale one. ok=false on failure (idle-suspend is best-effort, so callers skip).
// 0660 matches nginx's writer uid on the access socket and the UI stream socket.
func listenDatagram(path string) (net.PacketConn, bool) {
	if err := os.MkdirAll(config.RunDir(), 0755); err != nil {
		return nil, false
	}
	_ = os.Remove(path)
	conn, err := net.ListenPacket("unixgram", path)
	if err != nil {
		return nil, false
	}
	_ = os.Chmod(path, 0660)
	return conn, true
}

// readDatagrams delivers each datagram to handle until the socket closes (daemon
// shutdown), at which point ReadFrom errors and the loop exits.
func readDatagrams(conn net.PacketConn, handle func([]byte)) {
	buf := make([]byte, 4096)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		handle(buf[:n])
	}
}

// resolveHostToSite maps a request host to its idle key. A worktree domain
// resolves to the worktree's key (so its own traffic wakes the worktree, not the
// parent site); other hosts resolve to the owning site name. Hosts that belong to
// no registered site resolve to ok=false and are ignored by the tracker.
func resolveHostToSite(host string) (string, bool) {
	if key := idleEng.worktreeKeyForHost(host); key != "" {
		return key, true
	}
	site, err := config.FindSiteByDomain(host)
	if err != nil || site == nil {
		return "", false
	}
	return site.Name, true
}

// seedActiveSites restores each site's last-active time on startup: from the
// persisted file when present (so a restart/deploy keeps the countdown going),
// otherwise seeded to now (a new or never-seen site gets the grace window rather
// than looking instantly idle).
func seedActiveSites(t *idle.Tracker) {
	saved := idle.LoadActivity(config.IdleActivityFile())
	reg, err := config.LoadSites()
	if err != nil {
		return
	}
	now := time.Now()
	for _, s := range reg.Sites {
		if ts, ok := saved[s.Name]; ok && ts > 0 {
			t.TouchSite(s.Name, time.Unix(ts, 0))
		} else {
			t.TouchSite(s.Name, now)
		}
	}
	// Restore persisted worktree countdowns too (their keys carry a "/"), so a
	// restart doesn't hand every worktree a fresh grace window. A stale key for a
	// removed worktree is harmless: the engine only ever acts on worktrees it
	// re-detects from disk.
	for key, ts := range saved {
		if ts > 0 && strings.IndexByte(key, '/') >= 0 {
			t.TouchSite(key, time.Unix(ts, 0))
		}
	}
}
