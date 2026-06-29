package watcher

import (
	"errors"
	"testing"
	"time"
)

// TestTickDNS pins the publish-on-transition behaviour. A regression
// that drops a publish would silently break post-boot dashboard
// staleness recovery without any other test catching it.
func TestTickDNS(t *testing.T) {
	type result struct {
		checks    int
		waits     int
		repairs   int
		publishes int
		logs      []string
	}

	cases := []struct {
		name string
		// state going in
		lastOK    *bool
		tickCount int
		// dependency outcomes for this tick
		idleOrLocked bool
		checkOK      bool
		waitErr      error
		repairErr    error
		// expectations
		wantChecks      int
		wantWaits       int
		wantRepairs     int
		wantPublishes   int
		wantLogs        int
		wantLogKinds    []string // levels in order
		wantLastOKAfter *bool
	}{
		{
			// Boot case: dashboard rendered dns.ok=false from the cold
			// snapshot. First observation must publish so the UI catches
			// up without a manual refresh.
			name:            "first tick up publishes",
			lastOK:          nil,
			checkOK:         true,
			wantChecks:      1,
			wantPublishes:   1,
			wantLastOKAfter: ptrBool(true),
		},
		{
			// First tick observes DNS down. Repair pipeline runs; a
			// successful repair publishes once for the down observation
			// and once for the post-repair up flip. Two publishes total.
			name:            "first tick down with successful repair publishes twice",
			lastOK:          nil,
			checkOK:         false,
			wantChecks:      1,
			wantWaits:       1,
			wantRepairs:     1,
			wantPublishes:   2,
			wantLogs:        2,
			wantLogKinds:    []string{"warn", "info"},
			wantLastOKAfter: ptrBool(true),
		},
		{
			// Steady state: nothing changed since last tick, no publish.
			// This is the bulk of ticks on a healthy machine and the path
			// that has to stay quiet to keep CPU near zero.
			name:            "steady up no publish",
			lastOK:          ptrBool(true),
			checkOK:         true,
			wantChecks:      1,
			wantPublishes:   0,
			wantLastOKAfter: ptrBool(true),
		},
		{
			// DNS broke between ticks. Publish the down transition, then
			// repair succeeds and publishes the up flip.
			name:            "up to down with successful repair publishes twice",
			lastOK:          ptrBool(true),
			checkOK:         false,
			wantChecks:      1,
			wantWaits:       1,
			wantRepairs:     1,
			wantPublishes:   2,
			wantLogs:        2,
			wantLogKinds:    []string{"warn", "info"},
			wantLastOKAfter: ptrBool(true),
		},
		{
			// DNS broke and the repair pipeline can't even confirm
			// lerd-dns is ready. Publish the down transition only — we
			// haven't restored anything, so don't lie about up.
			name:            "down with wait failure publishes once",
			lastOK:          ptrBool(true),
			checkOK:         false,
			waitErr:         errors.New("timeout"),
			wantChecks:      1,
			wantWaits:       1,
			wantRepairs:     0,
			wantPublishes:   1,
			wantLogs:        2,
			wantLogKinds:    []string{"warn", "error"},
			wantLastOKAfter: ptrBool(false),
		},
		{
			// Repair attempt actually ran but configureResolver errored.
			// Same outcome as wait failure for the publish count: down
			// transition only, no up flip.
			name:            "down with repair failure publishes once",
			lastOK:          ptrBool(true),
			checkOK:         false,
			repairErr:       errors.New("write resolv.conf: permission denied"),
			wantChecks:      1,
			wantWaits:       1,
			wantRepairs:     1,
			wantPublishes:   1,
			wantLogs:        2,
			wantLogKinds:    []string{"warn", "error"},
			wantLastOKAfter: ptrBool(false),
		},
		{
			// Steady-down: prior was already down, only the post-repair
			// up flip publishes; no down-transition publish since nothing
			// transitioned on the down side.
			name:            "steady down with successful repair publishes once",
			lastOK:          ptrBool(false),
			checkOK:         false,
			wantChecks:      1,
			wantWaits:       1,
			wantRepairs:     1,
			wantPublishes:   1,
			wantLogs:        2,
			wantLogKinds:    []string{"warn", "info"},
			wantLastOKAfter: ptrBool(true),
		},
		{
			// Idle-skip path: laptop locked, this is not the every-Nth
			// tick, watcher must do nothing — no probe, no publish, no
			// state mutation. This is the battery-saving guarantee.
			name:            "idle skipped tick is silent",
			lastOK:          ptrBool(true),
			tickCount:       0, // becomes 1 after increment, 1 % 10 != 0 -> skip
			idleOrLocked:    true,
			wantChecks:      0,
			wantPublishes:   0,
			wantLastOKAfter: ptrBool(true),
		},
		{
			// Idle-but-Nth-tick: must run a real probe even on a locked
			// session so a returning user hits a healed DNS without the
			// 0.5–1s resolver timeout that prompted idleSkipEveryN.
			name:            "idle Nth tick still probes",
			lastOK:          ptrBool(true),
			tickCount:       9, // becomes 10, 10 % 10 == 0 -> probe
			idleOrLocked:    true,
			checkOK:         true,
			wantChecks:      1,
			wantPublishes:   0,
			wantLastOKAfter: ptrBool(true),
		},
		{
			// Natural recovery: lerd-dns came up between ticks without
			// a repair. Publish the transition, take no other action.
			name:            "down to up natural recovery publishes",
			lastOK:          ptrBool(false),
			checkOK:         true,
			wantChecks:      1,
			wantPublishes:   1,
			wantLastOKAfter: ptrBool(true),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got result
			deps := dnsWatchDeps{
				check: func(string) (bool, error) {
					got.checks++
					return c.checkOK, nil
				},
				waitReady: func(time.Duration) error {
					got.waits++
					return c.waitErr
				},
				configureResolver: func() error {
					got.repairs++
					return c.repairErr
				},
				idleOrLocked: func() bool { return c.idleOrLocked },
				publishStatus: func() {
					got.publishes++
				},
				log: func(level, _ string, _ ...any) {
					got.logs = append(got.logs, level)
				},
			}
			state := &dnsWatchState{lastOK: c.lastOK, tickCount: c.tickCount}
			tickDNS(deps, state, "test", false)

			if got.checks != c.wantChecks {
				t.Errorf("check() calls=%d, want %d", got.checks, c.wantChecks)
			}
			if got.waits != c.wantWaits {
				t.Errorf("waitReady() calls=%d, want %d", got.waits, c.wantWaits)
			}
			if got.repairs != c.wantRepairs {
				t.Errorf("configureResolver() calls=%d, want %d", got.repairs, c.wantRepairs)
			}
			if got.publishes != c.wantPublishes {
				t.Errorf("publishes=%d, want %d", got.publishes, c.wantPublishes)
			}
			if len(got.logs) != c.wantLogs {
				t.Errorf("logs=%d, want %d (%v)", len(got.logs), c.wantLogs, got.logs)
			}
			for i, want := range c.wantLogKinds {
				if i >= len(got.logs) {
					t.Errorf("missing log[%d]: want %q", i, want)
					continue
				}
				if got.logs[i] != want {
					t.Errorf("log[%d]=%q, want %q", i, got.logs[i], want)
				}
			}
			if !ptrBoolEq(state.lastOK, c.wantLastOKAfter) {
				t.Errorf("lastOK after tick=%v, want %v", deref(state.lastOK), deref(c.wantLastOKAfter))
			}
		})
	}
}

// TestTickDNS_repairUnavailable_logsOnceAndSkipsResolverWrite pins the
// macOS-without-sudoers fix: when repairPossible() returns false, the
// watcher must not call configureResolver (which would prompt for a sudo
// password from a non-interactive systemd / launchd context and spam the
// log every tick). Instead, log exactly once until the gate opens again.
func TestTickDNS_repairUnavailable_logsOnceAndSkipsResolverWrite(t *testing.T) {
	var checks, repairs, waits int
	var logs []string

	deps := dnsWatchDeps{
		check:             func(string) (bool, error) { checks++; return false, nil },
		waitReady:         func(time.Duration) error { waits++; return nil },
		configureResolver: func() error { repairs++; return nil },
		repairPossible:    func() bool { return false },
		idleOrLocked:      func() bool { return false },
		publishStatus:     func() {},
		log: func(level, _ string, _ ...any) {
			logs = append(logs, level)
		},
	}
	state := &dnsWatchState{}

	// Three consecutive ticks with DNS down; only the first should log.
	for i := 0; i < 3; i++ {
		tickDNS(deps, state, "test", false)
	}

	if checks != 3 {
		t.Errorf("expected 3 checks, got %d", checks)
	}
	if waits != 0 {
		t.Errorf("expected zero waitReady calls when repair is unavailable, got %d", waits)
	}
	if repairs != 0 {
		t.Errorf("expected zero configureResolver calls when repair is unavailable, got %d", repairs)
	}
	// Two log entries: first publish-on-transition (no log) then a single
	// "repair unavailable" warn. Subsequent ticks must not re-log.
	if len(logs) != 1 {
		t.Errorf("expected 1 log line over 3 ticks, got %d (%v)", len(logs), logs)
	}
}

// TestTickDNS_repairAvailableAfterUnavailable_relogsOnNextOutage pins the
// reset behaviour: once DNS comes back up (or the gate opens), the
// "repair unavailable" warn flag clears so the next outage logs again.
func TestTickDNS_repairAvailableAfterUnavailable_relogsOnNextOutage(t *testing.T) {
	gate := false
	var logs []string
	deps := dnsWatchDeps{
		check:             func(string) (bool, error) { return false, nil },
		waitReady:         func(time.Duration) error { return errors.New("not relevant") },
		configureResolver: func() error { return nil },
		repairPossible:    func() bool { return gate },
		idleOrLocked:      func() bool { return false },
		publishStatus:     func() {},
		log:               func(level, _ string, _ ...any) { logs = append(logs, level) },
	}
	state := &dnsWatchState{}

	// Tick with gate closed: one warn.
	tickDNS(deps, state, "test", false)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d (%v)", len(logs), logs)
	}
	logs = nil

	// Open the gate: next tick should run the normal repair pipeline,
	// which logs warn + error (waitReady fails). Importantly the
	// repairUnavailable flag must be cleared so a future close-of-gate
	// re-emits the once warn.
	gate = true
	tickDNS(deps, state, "test", false)
	if len(logs) < 1 {
		t.Fatalf("expected repair pipeline logs after gate reopened, got %v", logs)
	}
	if state.repairUnavailable {
		t.Error("expected repairUnavailable flag to clear when gate reopens")
	}

	// Close gate again — should re-log the once-warn.
	logs = nil
	gate = false
	tickDNS(deps, state, "test", false)
	if len(logs) != 1 {
		t.Errorf("expected re-log after gate re-closes, got %v", logs)
	}
}

// TestTickDNS_containerDNSResync pins the VPN fix: the watcher records the
// host DNS fingerprint as a baseline on its first tick, stays quiet while
// it is unchanged, and re-syncs container DNS exactly once when it changes
// (a VPN connecting), then settles again.
func TestTickDNS_containerDNSResync(t *testing.T) {
	fp := "1.1.1.1|0"
	resyncs := 0
	deps := dnsWatchDeps{
		check:              func(string) (bool, error) { return true, nil },
		idleOrLocked:       func() bool { return false },
		publishStatus:      func() {},
		dnsEnvFingerprint:  func() string { return fp },
		resyncContainerDNS: func() error { resyncs++; return nil },
		log:                func(string, string, ...any) {},
	}
	state := &dnsWatchState{}

	// First tick only records the baseline.
	tickDNS(deps, state, "test", false)
	if resyncs != 0 {
		t.Fatalf("first tick re-synced; want baseline only, got %d", resyncs)
	}
	// Unchanged environment: still quiet.
	tickDNS(deps, state, "test", false)
	if resyncs != 0 {
		t.Fatalf("unchanged env re-synced=%d, want 0", resyncs)
	}
	// VPN connects: the fingerprint changes, re-sync fires once.
	fp = "1.1.1.1,10.0.0.1|1"
	tickDNS(deps, state, "test", false)
	if resyncs != 1 {
		t.Fatalf("changed env re-syncs=%d, want 1", resyncs)
	}
	// Stable at the new value: no further re-sync.
	tickDNS(deps, state, "test", false)
	if resyncs != 1 {
		t.Fatalf("re-syncs after settle=%d, want 1", resyncs)
	}
}

// TestTickDNS_exposeMappingRepair pins the lan:expose heal: when DNS reads
// down because the published .tld mapping drifted from the host's current LAN
// IP (sleep/wake DHCP renew), the watcher must re-render the mapping and
// reload lerd-dns *before* the sudo-gated resolver repair, then re-check and
// publish the recovery. A no-op or a failed re-render must fall through to the
// existing resolver repair so neither path is shadowed.
func TestTickDNS_exposeMappingRepair(t *testing.T) {
	t.Run("re-render heals and skips resolver repair", func(t *testing.T) {
		var checks, exposeCalls, resolverCalls, waits, publishes int
		var logs []string
		deps := dnsWatchDeps{
			// First probe down, second (post re-render) up.
			check: func(string) (bool, error) {
				checks++
				return checks > 1, nil
			},
			waitReady:           func(time.Duration) error { waits++; return nil },
			configureResolver:   func() error { resolverCalls++; return nil },
			repairExposeMapping: func() (bool, error) { exposeCalls++; return true, nil },
			idleOrLocked:        func() bool { return false },
			publishStatus:       func() { publishes++ },
			log:                 func(level, _ string, _ ...any) { logs = append(logs, level) },
		}
		state := &dnsWatchState{lastOK: ptrBool(true)}
		tickDNS(deps, state, "test", false)

		if exposeCalls != 1 {
			t.Errorf("expose repair calls=%d, want 1", exposeCalls)
		}
		if checks != 2 {
			t.Errorf("checks=%d, want 2 (probe + re-check after re-render)", checks)
		}
		if resolverCalls != 0 || waits != 0 {
			t.Errorf("resolver repair must not run after a successful re-render: waits=%d resolver=%d", waits, resolverCalls)
		}
		// down transition + post-heal up flip.
		if publishes != 2 {
			t.Errorf("publishes=%d, want 2", publishes)
		}
		if !ptrBoolEq(state.lastOK, ptrBool(true)) {
			t.Errorf("lastOK=%v, want true", deref(state.lastOK))
		}
		// info log for the re-render only (down transition doesn't log).
		if len(logs) != 1 || logs[0] != "info" {
			t.Errorf("logs=%v, want one info", logs)
		}
	})

	t.Run("no-op re-render falls through to resolver repair", func(t *testing.T) {
		var exposeCalls, resolverCalls, waits int
		deps := dnsWatchDeps{
			check:               func(string) (bool, error) { return false, nil },
			waitReady:           func(time.Duration) error { waits++; return nil },
			configureResolver:   func() error { resolverCalls++; return nil },
			repairExposeMapping: func() (bool, error) { exposeCalls++; return false, nil },
			idleOrLocked:        func() bool { return false },
			publishStatus:       func() {},
			log:                 func(string, string, ...any) {},
		}
		state := &dnsWatchState{lastOK: ptrBool(false)}
		tickDNS(deps, state, "test", false)

		if exposeCalls != 1 {
			t.Errorf("expose repair calls=%d, want 1", exposeCalls)
		}
		if waits != 1 || resolverCalls != 1 {
			t.Errorf("a no-op re-render must fall through to resolver repair: waits=%d resolver=%d", waits, resolverCalls)
		}
	})

	t.Run("re-render error logs warn and falls through", func(t *testing.T) {
		var resolverCalls int
		var logs []string
		deps := dnsWatchDeps{
			check:               func(string) (bool, error) { return false, nil },
			waitReady:           func(time.Duration) error { return nil },
			configureResolver:   func() error { resolverCalls++; return nil },
			repairExposeMapping: func() (bool, error) { return false, errors.New("reload lerd-dns: boom") },
			idleOrLocked:        func() bool { return false },
			publishStatus:       func() {},
			log:                 func(level, _ string, _ ...any) { logs = append(logs, level) },
		}
		state := &dnsWatchState{lastOK: ptrBool(false)}
		tickDNS(deps, state, "test", false)

		if resolverCalls != 1 {
			t.Errorf("resolver repair must still run after a re-render error, got %d", resolverCalls)
		}
		if len(logs) < 1 || logs[0] != "warn" {
			t.Errorf("expected a leading warn for the re-render error, got %v", logs)
		}
	})
}

// TestRunDNSLoop_linkChangeKicksTick pins that an interface change wakes
// the watcher early, otherwise the user sees the dashboard pill stuck red
// for up to the full poll interval after a VPN connect or disconnect.
func TestRunDNSLoop_linkChangeKicksTick(t *testing.T) {
	ticks := make(chan struct{}, 16)
	deps := dnsWatchDeps{
		check:         func(string) (bool, error) { ticks <- struct{}{}; return true, nil },
		idleOrLocked:  func() bool { return false },
		publishStatus: func() {},
		log:           func(string, string, ...any) {},
	}
	state := &dnsWatchState{}
	tickerC := make(chan time.Time, 4)
	linkC := make(chan struct{}, 4)
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() { runDNSLoop(deps, state, "test", tickerC, linkC, done); close(stopped) }()
	defer func() { close(done); <-stopped }()

	select {
	case <-ticks:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("initial tick missing")
	}

	linkC <- struct{}{}
	select {
	case <-ticks:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("link change did not kick a tick")
	}

	tickerC <- time.Now()
	select {
	case <-ticks:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ticker fire did not trigger a tick")
	}
}

// TestRunDNSLoop_linkChangeKicksTickWhileIdle pins the whole point of the link
// watcher: a host network change must re-probe immediately even on an idle or
// locked session. The polling backoff (idleSkipEveryN) exists to save battery
// on the timer path, but a VPN connect/disconnect or wifi switch frequently
// happens while the laptop is asleep or on the lock screen, so routing the
// link-change tick through the same idle skip would drop ~9 of every 10 events
// and leave containers on the stale resolver until the next Nth poll.
func TestRunDNSLoop_linkChangeKicksTickWhileIdle(t *testing.T) {
	ticks := make(chan struct{}, 16)
	deps := dnsWatchDeps{
		check:         func(string) (bool, error) { ticks <- struct{}{}; return true, nil },
		idleOrLocked:  func() bool { return true },
		publishStatus: func() {},
		log:           func(string, string, ...any) {},
	}
	state := &dnsWatchState{}
	tickerC := make(chan time.Time, 4)
	linkC := make(chan struct{}, 4)
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() { runDNSLoop(deps, state, "test", tickerC, linkC, done); close(stopped) }()
	defer func() { close(done); <-stopped }()

	// The initial poll is skipped while idle (tickCount 1, not the Nth), so it
	// must not probe — confirm the loop is genuinely in the backoff state.
	select {
	case <-ticks:
		t.Fatal("idle initial poll probed; backoff should have skipped it")
	case <-time.After(100 * time.Millisecond):
	}

	// The link change must probe right away despite being idle.
	linkC <- struct{}{}
	select {
	case <-ticks:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("link change did not kick a tick while idle")
	}
}

func ptrBool(b bool) *bool { return &b }

func ptrBoolEq(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func deref(p *bool) any {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestSuperviseLinkChanges_RestartsOnTransientError(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	calls := make(chan int, 8)
	var n int
	fn := func(out chan<- struct{}, d <-chan struct{}) error {
		n++
		calls <- n
		if n < 3 {
			return errors.New("transient netlink failure")
		}
		<-d // the third attempt stays up until shutdown
		return nil
	}

	go superviseLinkChanges(fn, make(chan struct{}, 1), done, func(int) time.Duration { return 0 }, time.Minute)

	// We expect three attempts: two that error and restart, one that sticks.
	for want := 1; want <= 3; want++ {
		select {
		case got := <-calls:
			if got != want {
				t.Fatalf("attempt %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("attempt %d never happened", want)
		}
	}
}

func TestSuperviseLinkChanges_ResetsBackoffAfterHealthyRun(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	// Reset threshold of 15ms; the "healthy" run below sleeps 30ms before failing.
	attempts := make(chan int, 16)
	backoff := func(a int) time.Duration { attempts <- a; return 0 }

	var n int
	fn := func(out chan<- struct{}, d <-chan struct{}) error {
		n++
		switch {
		case n <= 3:
			return errors.New("rapid transient failure") // short runs grow the backoff
		case n == 4:
			time.Sleep(30 * time.Millisecond) // healthy run, then a late error
			return errors.New("late transient failure")
		default:
			<-d // stay up until shutdown
			return nil
		}
	}

	go superviseLinkChanges(fn, make(chan struct{}, 1), done, backoff, 15*time.Millisecond)

	// The three rapid failures grow the backoff attempt: 0, 1, 2.
	for want := 0; want <= 2; want++ {
		select {
		case got := <-attempts:
			if got != want {
				t.Fatalf("backoff attempt %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("backoff not called")
		}
	}
	// The fourth attempt ran longer than the reset threshold before failing, so
	// the next restart must reset the backoff to 0 rather than stay near the cap.
	select {
	case got := <-attempts:
		if got != 0 {
			t.Fatalf("backoff after a healthy run = %d, want 0 (reset)", got)
		}
	case <-time.After(time.Second):
		t.Fatal("backoff not called after the healthy run")
	}
}

func TestSuperviseLinkChanges_StopsAfterDone(t *testing.T) {
	done := make(chan struct{})
	close(done) // already shutting down

	var n int
	fn := func(out chan<- struct{}, d <-chan struct{}) error {
		n++
		return errors.New("read after shutdown")
	}

	stopped := make(chan struct{})
	go func() {
		superviseLinkChanges(fn, make(chan struct{}, 1), done, func(int) time.Duration { return 0 }, time.Minute)
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("supervisor did not return after done was closed")
	}
	if n != 1 {
		t.Fatalf("expected exactly one attempt with done closed, got %d", n)
	}
}

func TestExposeConfigChanged_IgnoresAAAAOnlyDrift(t *testing.T) {
	base := "# Lerd DNS configuration\nport=5300\nno-resolv\nserver=1.1.1.1\naddress=/.test/192.168.1.5\n"
	withV6 := base + "address=/.test/2001:db8::1\n"
	withOtherV6 := base + "address=/.test/2001:db8::2\n"

	// A v6 address appearing, rotating, or disappearing must not trigger a restart.
	if exposeConfigChanged([]byte(base), []byte(withV6)) {
		t.Fatal("adding an AAAA record should not warrant a restart")
	}
	if exposeConfigChanged([]byte(withV6), []byte(withOtherV6)) {
		t.Fatal("rotating the AAAA record should not warrant a restart")
	}
	if exposeConfigChanged([]byte(withV6), []byte(base)) {
		t.Fatal("dropping the AAAA record should not warrant a restart")
	}

	// A v4 drift (the real LAN IP change) must still trigger a restart.
	v4Changed := "# Lerd DNS configuration\nport=5300\nno-resolv\nserver=1.1.1.1\naddress=/.test/192.168.1.9\n"
	if !exposeConfigChanged([]byte(base), []byte(v4Changed)) {
		t.Fatal("a v4 address change must warrant a restart")
	}
	// An upstream change must still trigger a restart.
	upstreamChanged := "# Lerd DNS configuration\nport=5300\nno-resolv\nserver=8.8.8.8\naddress=/.test/192.168.1.5\n"
	if !exposeConfigChanged([]byte(base), []byte(upstreamChanged)) {
		t.Fatal("an upstream change must warrant a restart")
	}
}

// fakeClock returns controllable times so resume detection (a wall-clock gap
// between ticks) can be exercised deterministically.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// nginxHarness builds a healNginxOnResume test harness with healthy defaults so
// each test overrides only the behaviour it pins.
type nginxHarness struct {
	deps    dnsWatchDeps
	repairs int
	healthy bool
	stopped bool
}

func newNginxHarness() *nginxHarness {
	h := &nginxHarness{healthy: true}
	h.deps = dnsWatchDeps{
		nginxHealthy: func() bool { return h.healthy },
		repairNginx:  func() error { h.repairs++; return nil },
		isStopped:    func() bool { return h.stopped },
		log:          func(string, string, ...any) {},
	}
	return h
}

// TestResumed pins the suspend detector: a wall-clock gap far larger than the
// poll interval reads as a resume, normal cadence does not, and the very first
// tick (no previous timestamp) never does.
func TestResumed(t *testing.T) {
	c := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := &dnsWatchState{}
	if s.resumed(c.now()) {
		t.Fatal("first tick must not read as a resume")
	}
	c.advance(30 * time.Second)
	if s.resumed(c.now()) {
		t.Fatal("a normal poll-interval gap is not a resume")
	}
	c.advance(2 * time.Hour)
	if !s.resumed(c.now()) {
		t.Fatal("a multi-hour gap must read as a resume")
	}
	c.advance(30 * time.Second)
	if s.resumed(c.now()) {
		t.Fatal("back to normal cadence must not read as a resume")
	}
}

// TestHealNginxOnResume: a down nginx is restarted, a healthy one is left alone, a
// deliberately stopped stack is never touched, and an unset probe is a no-op.
func TestHealNginxOnResume(t *testing.T) {
	h := newNginxHarness()
	h.healthy = false
	healNginxOnResume(h.deps)
	if h.repairs != 1 {
		t.Fatalf("down nginx must restart on resume, repairs=%d", h.repairs)
	}

	h = newNginxHarness() // healthy
	healNginxOnResume(h.deps)
	if h.repairs != 0 {
		t.Fatalf("healthy nginx must not restart, repairs=%d", h.repairs)
	}

	h = newNginxHarness()
	h.healthy = false
	h.stopped = true
	healNginxOnResume(h.deps)
	if h.repairs != 0 {
		t.Fatalf("stopped stack must not be resurrected, repairs=%d", h.repairs)
	}

	healNginxOnResume(dnsWatchDeps{log: func(string, string, ...any) {}}) // nil probe: no panic, no-op
}

// TestTickDNS_resumeTriggersNginxHeal: nginx is probed and restarted only on the
// tick that detects a resume — never on the first tick or a normal-cadence tick,
// so a `lerd start` (which a fresh watcher never reads as a resume) can't trip it.
func TestTickDNS_resumeTriggersNginxHeal(t *testing.T) {
	c := &fakeClock{t: time.Unix(1_000_000, 0)}
	probes, restarts := 0, 0
	deps := dnsWatchDeps{
		check:         func(string) (bool, error) { return true, nil },
		idleOrLocked:  func() bool { return false },
		publishStatus: func() {},
		now:           c.now,
		nginxHealthy:  func() bool { probes++; return false },
		repairNginx:   func() error { restarts++; return nil },
		log:           func(string, string, ...any) {},
	}
	s := &dnsWatchState{}

	tickDNS(deps, s, "test", false) // first tick: no prior time, not a resume
	c.advance(30 * time.Second)
	tickDNS(deps, s, "test", false) // normal gap: not a resume
	if probes != 0 {
		t.Fatalf("non-resume ticks must not probe nginx, got %d", probes)
	}

	c.advance(2 * time.Hour)
	tickDNS(deps, s, "test", false) // big gap: resume -> heal
	if probes != 1 || restarts != 1 {
		t.Fatalf("resume must probe and restart a down nginx: probes=%d restarts=%d", probes, restarts)
	}
}

// TestTickDNS_resumeHealsEvenWhenIdle: an idle/locked poll tick backs off the DNS
// probe, but a detected resume must still heal nginx.
func TestTickDNS_resumeHealsEvenWhenIdle(t *testing.T) {
	c := &fakeClock{t: time.Unix(1_000_000, 0)}
	restarts := 0
	deps := dnsWatchDeps{
		check:         func(string) (bool, error) { return true, nil },
		idleOrLocked:  func() bool { return true },
		publishStatus: func() {},
		now:           c.now,
		nginxHealthy:  func() bool { return false },
		repairNginx:   func() error { restarts++; return nil },
		log:           func(string, string, ...any) {},
	}
	s := &dnsWatchState{tickCount: 3} // idle, non-Nth tick -> DNS would back off
	tickDNS(deps, s, "test", false)
	c.advance(2 * time.Hour)
	tickDNS(deps, s, "test", false)
	if restarts != 1 {
		t.Fatalf("a resume must heal nginx even on an idle tick, restarts=%d", restarts)
	}
}

// TestTickDNS_stoppedTickDoesNothing: after `lerd stop` the watcher must act on
// nothing — not the DNS repair, not even a resume-triggered nginx restart.
func TestTickDNS_stoppedTickDoesNothing(t *testing.T) {
	c := &fakeClock{t: time.Unix(1_000_000, 0)}
	waits, restarts := 0, 0
	deps := dnsWatchDeps{
		check:             func(string) (bool, error) { return false, nil },
		idleOrLocked:      func() bool { return false },
		publishStatus:     func() {},
		repairPossible:    func() bool { return true },
		waitReady:         func(time.Duration) error { waits++; return nil },
		configureResolver: func() error { return nil },
		now:               c.now,
		nginxHealthy:      func() bool { return false },
		repairNginx:       func() error { restarts++; return nil },
		isStopped:         func() bool { return true },
		log:               func(string, string, ...any) {},
	}
	s := &dnsWatchState{}
	tickDNS(deps, s, "test", false)
	c.advance(2 * time.Hour) // even a resume-sized gap
	tickDNS(deps, s, "test", false)
	if waits != 0 || restarts != 0 {
		t.Fatalf("stopped stack must do nothing: waits=%d restarts=%d", waits, restarts)
	}
}
