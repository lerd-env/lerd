package watcher

import (
	"errors"
	"testing"
)

// tickHostGateway is the decision point for the host-gateway watcher.
// These table-driven tests pin its states so a future refactor that
// breaks any of them (e.g. rewriting when it shouldn't, or failing to
// rewrite when it should) fails loudly in CI.
func TestTickHostGateway(t *testing.T) {
	type result struct {
		wrote             bool
		reachableCalls    int
		detectFreshCalled bool
		logs              []string
	}

	cases := []struct {
		name                  string
		lastLAN               string
		currentLAN            string
		current               string
		reachable             bool
		fresh                 string
		writeErr              error
		wantWrote             bool
		wantReachableCalls    int
		wantDetectFreshCalled bool
		wantLogs              int
		wantLogKind           string // "info" or "warn" if a log was emitted
		wantLastLANAfterTick  string
	}{
		{
			// Fast-path: the common case on a stationary machine. LAN IP
			// is unchanged from the last tick, so we short-circuit before
			// touching podman. This is the ~99.99% path on a desktop and
			// the whole reason the optimization exists — a podman exec
			// per tick would burn 1-3 % CPU on macOS (gvproxy hop costs
			// ~300 ms – 1 s per exec).
			name:                 "lan unchanged, fast path",
			lastLAN:              "192.168.1.10",
			currentLAN:           "192.168.1.10",
			wantWrote:            false,
			wantReachableCalls:   0, // must NOT call reachable() — that's a podman exec
			wantLogs:             0,
			wantLastLANAfterTick: "192.168.1.10",
		},
		{
			// LAN changed but the old /etc/hosts entry still reaches the
			// host (e.g. moved VPNs but the old probe address is still
			// routable). No rewrite, but we did pay for the podman exec
			// because the LAN change triggered the slow path.
			name:                 "lan changed, current still reachable",
			lastLAN:              "192.168.1.10",
			currentLAN:           "10.0.0.50",
			current:              "192.168.1.10",
			reachable:            true,
			wantWrote:            false,
			wantReachableCalls:   1,
			wantLogs:             0,
			wantLastLANAfterTick: "10.0.0.50",
		},
		{
			// Coffee-shop case, the whole reason the watcher exists:
			// laptop moved networks, old IP no longer routes, probe
			// finds a new working one, watcher rewrites /etc/hosts and
			// Xdebug starts working again without user action.
			name:                  "lan changed, stale entry, probe finds new",
			lastLAN:               "192.168.1.10",
			currentLAN:            "10.0.0.50",
			current:               "192.168.1.10",
			reachable:             false,
			fresh:                 "10.0.0.50",
			wantWrote:             true,
			wantReachableCalls:    1,
			wantDetectFreshCalled: true,
			wantLogs:              1, wantLogKind: "info",
			wantLastLANAfterTick: "10.0.0.50",
		},
		{
			// LAN changed but the laptop is offline or lerd-nginx is
			// down between ticks: probe returns "". Must NOT overwrite
			// with the legacy fallback — that would make things worse.
			// Try again next tick.
			name:                  "lan changed, probe finds nothing",
			lastLAN:               "192.168.1.10",
			currentLAN:            "10.0.0.50",
			current:               "192.168.1.10",
			reachable:             false,
			fresh:                 "",
			wantWrote:             false,
			wantReachableCalls:    1,
			wantDetectFreshCalled: true,
			wantLogs:              0,
			wantLastLANAfterTick:  "10.0.0.50",
		},
		{
			// Regression: probe reports the same IP already on disk (can
			// happen on macOS where gvproxy's address doesn't depend on
			// LAN IP). Skip the write so we don't thrash the bind-mounted
			// file and trigger spurious inotify events.
			name:                  "lan changed but probe confirms current",
			lastLAN:               "192.168.1.10",
			currentLAN:            "10.0.0.50",
			current:               "192.168.127.254", // gvproxy address
			reachable:             false,
			fresh:                 "192.168.127.254",
			wantWrote:             false,
			wantReachableCalls:    1,
			wantDetectFreshCalled: true,
			wantLogs:              0,
			wantLastLANAfterTick:  "10.0.0.50",
		},
		{
			// Write fails mid-flight. Log warn, advance last-known LAN
			// anyway so we don't spin on the same failure every tick.
			name:                  "write error is surfaced",
			lastLAN:               "192.168.1.10",
			currentLAN:            "10.0.0.50",
			current:               "192.168.1.10",
			reachable:             false,
			fresh:                 "10.0.0.50",
			writeErr:              errors.New("disk full"),
			wantWrote:             true,
			wantReachableCalls:    1,
			wantDetectFreshCalled: true,
			wantLogs:              1, wantLogKind: "warn",
			wantLastLANAfterTick: "10.0.0.50",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got result
			deps := hostGatewayDeps{
				primaryLANIP: func() string { return c.currentLAN },
				readCurrent:  func() string { return c.current },
				reachable: func(ip string) bool {
					got.reachableCalls++
					return c.reachable && ip == c.current
				},
				detectFresh: func() string {
					got.detectFreshCalled = true
					return c.fresh
				},
				writeHosts: func() error {
					got.wrote = true
					return c.writeErr
				},
				// Gateway-only cases: pin nginx to a matching pair so the
				// nginx half of the tick is a no-op and can't skew the counts.
				readNginxIP:  func() string { return "10.89.7.11" },
				freshNginxIP: func() string { return "10.89.7.11" },
				log: func(level, _ string, _ ...any) {
					got.logs = append(got.logs, level)
				},
			}
			state := &hostGatewayState{lastLAN: c.lastLAN}
			tickHostGateway(deps, state)

			if got.wrote != c.wantWrote {
				t.Errorf("wrote=%v, want %v", got.wrote, c.wantWrote)
			}
			if got.reachableCalls != c.wantReachableCalls {
				t.Errorf("reachable() calls=%d, want %d", got.reachableCalls, c.wantReachableCalls)
			}
			if got.detectFreshCalled != c.wantDetectFreshCalled {
				t.Errorf("detectFresh() called=%v, want %v", got.detectFreshCalled, c.wantDetectFreshCalled)
			}
			if len(got.logs) != c.wantLogs {
				t.Errorf("logs=%d, want %d (%v)", len(got.logs), c.wantLogs, got.logs)
			}
			if c.wantLogs > 0 && len(got.logs) > 0 && got.logs[0] != c.wantLogKind {
				t.Errorf("log kind=%q, want %q", got.logs[0], c.wantLogKind)
			}
			if state.lastLAN != c.wantLastLANAfterTick {
				t.Errorf("lastLAN after tick=%q, want %q", state.lastLAN, c.wantLastLANAfterTick)
			}
		})
	}
}

// onUpdate must fire only after the hosts file is actually rewritten, so
// host-proxy vhosts are regenerated on a real gateway change and not on the
// fast-path skip or a reachable-current no-op.
func TestTickHostGateway_onUpdateFiresOnlyOnRewrite(t *testing.T) {
	base := func(onUpdate func()) hostGatewayDeps {
		return hostGatewayDeps{
			primaryLANIP: func() string { return "10.0.0.50" },
			readCurrent:  func() string { return "192.168.1.10" },
			reachable:    func(string) bool { return false },
			detectFresh:  func() string { return "10.0.0.50" },
			writeHosts:   func() error { return nil },
			readNginxIP:  func() string { return "10.89.7.11" },
			freshNginxIP: func() string { return "10.89.7.11" },
			onUpdate:     onUpdate,
			log:          func(string, string, ...any) {},
		}
	}

	t.Run("fires on rewrite", func(t *testing.T) {
		called := false
		deps := base(func() { called = true })
		tickHostGateway(deps, &hostGatewayState{lastLAN: "192.168.1.10"})
		if !called {
			t.Error("onUpdate should fire after a successful hosts rewrite")
		}
	})

	t.Run("skips when LAN unchanged", func(t *testing.T) {
		called := false
		deps := base(func() { called = true })
		tickHostGateway(deps, &hostGatewayState{lastLAN: "10.0.0.50"})
		if called {
			t.Error("onUpdate must not fire on the fast-path skip")
		}
	})

	t.Run("skips when write fails", func(t *testing.T) {
		called := false
		deps := base(func() { called = true })
		deps.writeHosts = func() error { return errors.New("disk full") }
		tickHostGateway(deps, &hostGatewayState{lastLAN: "192.168.1.10"})
		if called {
			t.Error("onUpdate must not fire when the hosts rewrite fails")
		}
	})
}

// Podman hands lerd-nginx a fresh bridge IP on every recreation. A reboot
// starts the quadlet units without `lerd start`, so nothing rewrote the hosts
// files and containers resolved .test domains to a dead address (issue #817).
func TestTickHostGateway_NginxIP(t *testing.T) {
	cases := []struct {
		name        string
		onDisk      string
		fresh       string
		writeErr    error
		wantWrote   bool
		wantLogs    int
		wantLogKind string
	}{
		{
			// The reboot case. nginx came back on a new IP, the file still
			// carries the old one, so the watcher rewrites both hosts files.
			name:      "nginx recreated with a new IP",
			onDisk:    "10.89.7.143",
			fresh:     "10.89.7.11",
			wantWrote: true,
			wantLogs:  1, wantLogKind: "info",
		},
		{
			// Steady state, the ~99.99% path. Must not thrash the
			// bind-mounted file or containers see spurious inotify events.
			name:      "nginx IP unchanged",
			onDisk:    "10.89.7.11",
			fresh:     "10.89.7.11",
			wantWrote: false,
		},
		{
			// nginx is down or being recreated between ticks. Writing now
			// would bake in the 127.0.0.1 fallback and break every container
			// until the next `lerd start`. Wait for the next tick instead.
			name:      "nginx not running",
			onDisk:    "10.89.7.11",
			fresh:     "",
			wantWrote: false,
		},
		{
			// No sites linked, so the file has no entry to compare against.
			// Nothing is stale and there is nothing to fix.
			name:      "no site entries on disk",
			onDisk:    "",
			fresh:     "10.89.7.11",
			wantWrote: false,
		},
		{
			name:      "write error is surfaced",
			onDisk:    "10.89.7.143",
			fresh:     "10.89.7.11",
			writeErr:  errors.New("disk full"),
			wantWrote: true,
			wantLogs:  1, wantLogKind: "warn",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var wrote bool
			var logs []string
			deps := hostGatewayDeps{
				// Park the gateway half on its fast path so any write we
				// observe can only have come from the nginx half.
				primaryLANIP: func() string { return "192.168.1.10" },
				readCurrent:  func() string { return "192.168.1.10" },
				reachable:    func(string) bool { return true },
				detectFresh:  func() string { return "192.168.1.10" },
				readNginxIP:  func() string { return c.onDisk },
				freshNginxIP: func() string { return c.fresh },
				writeHosts: func() error {
					wrote = true
					return c.writeErr
				},
				log: func(level, _ string, _ ...any) { logs = append(logs, level) },
			}
			tickHostGateway(deps, &hostGatewayState{lastLAN: "192.168.1.10"})

			if wrote != c.wantWrote {
				t.Errorf("wrote=%v, want %v", wrote, c.wantWrote)
			}
			if len(logs) != c.wantLogs {
				t.Errorf("logs=%d, want %d (%v)", len(logs), c.wantLogs, logs)
			}
			if c.wantLogs > 0 && len(logs) > 0 && logs[0] != c.wantLogKind {
				t.Errorf("log kind=%q, want %q", logs[0], c.wantLogKind)
			}
		})
	}
}

// The nginx IP is not baked into host-proxy vhosts, only the gateway IP is,
// so an nginx-only rewrite must not trigger a vhost regeneration.
func TestTickHostGateway_NginxRewriteSkipsOnUpdate(t *testing.T) {
	called := false
	deps := hostGatewayDeps{
		primaryLANIP: func() string { return "192.168.1.10" },
		readCurrent:  func() string { return "192.168.1.10" },
		reachable:    func(string) bool { return true },
		detectFresh:  func() string { return "192.168.1.10" },
		readNginxIP:  func() string { return "10.89.7.143" },
		freshNginxIP: func() string { return "10.89.7.11" },
		writeHosts:   func() error { return nil },
		onUpdate:     func() { called = true },
		log:          func(string, string, ...any) {},
	}
	tickHostGateway(deps, &hostGatewayState{lastLAN: "192.168.1.10"})
	if called {
		t.Error("onUpdate must not fire for an nginx-only rewrite")
	}
}
