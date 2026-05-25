package ui

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// setupConfigDir points config.LoadGlobal at a temp dir, optionally writing
// a config.yaml with the given UI credentials. When credentials are
// provided, lan.exposed is also set to true: the gate now treats LAN
// exposure as a top-level flag, so credentials without lan:expose result
// in 403 (which is correct production behavior but would break every
// existing "non-loopback with valid auth → 200" test). Tests that
// specifically want to verify the LAN-off-with-creds path should call
// setupConfigDirRaw directly.
func setupConfigDir(t *testing.T, username, plainPassword string) {
	t.Helper()
	setupConfigDirRaw(t, username, plainPassword, username != "" || plainPassword != "")
}

func setupConfigDirRaw(t *testing.T, username, plainPassword string, lanExposed bool) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg := map[string]any{}
	if lanExposed {
		cfg["lan"] = map[string]any{"exposed": true}
	}
	if username != "" || plainPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.MinCost)
		if err != nil {
			t.Fatalf("bcrypt: %v", err)
		}
		cfg["ui"] = map[string]any{
			"username":      username,
			"password_hash": string(hash),
		}
	}
	if len(cfg) == 0 {
		return
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	dir := filepath.Join(tmp, "lerd")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// nextHandler is a stub downstream handler that records whether it was
// called and writes a 200 OK with a marker body.
type nextHandler struct {
	called bool
}

func (n *nextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n.called = true
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func TestRemoteControlGate_loopbackBypassesEverything(t *testing.T) {
	setupConfigDir(t, "alice", "s3cret")

	next := &nextHandler{}
	gate := withRemoteControlGate(next)

	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if !next.called {
		t.Error("loopback request did not reach next handler")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("loopback status = %d, want 200", rec.Code)
	}
}

func TestRemoteControlGate_lanForbiddenWhenDisabled(t *testing.T) {
	setupConfigDir(t, "", "") // no auth configured

	next := &nextHandler{}
	gate := withRemoteControlGate(next)

	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	req.RemoteAddr = "192.168.1.42:54321"
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if next.called {
		t.Error("LAN request reached next handler when remote-control is off")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("LAN status = %d, want 403", rec.Code)
	}
}

func TestRemoteControlGate_lanRequiresAuthWhenEnabled(t *testing.T) {
	setupConfigDir(t, "alice", "s3cret")

	next := &nextHandler{}
	gate := withRemoteControlGate(next)

	t.Run("no header → 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
		req.RemoteAddr = "192.168.1.42:54321"
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
		if rec.Header().Get("WWW-Authenticate") == "" {
			t.Error("missing WWW-Authenticate header")
		}
	})

	t.Run("wrong user → 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
		req.RemoteAddr = "192.168.1.42:54321"
		req.SetBasicAuth("bob", "s3cret")
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("wrong password → 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
		req.RemoteAddr = "192.168.1.42:54321"
		req.SetBasicAuth("alice", "wrong")
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("correct creds → 200", func(t *testing.T) {
		next2 := &nextHandler{}
		gate2 := withRemoteControlGate(next2)
		req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
		req.RemoteAddr = "192.168.1.42:54321"
		req.SetBasicAuth("alice", "s3cret")
		rec := httptest.NewRecorder()
		gate2.ServeHTTP(rec, req)
		if !next2.called {
			t.Error("authorized LAN request did not reach next handler")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})
}

// TestRemoteControlGate_lanOffOverridesCredentials verifies the top-level
// LAN-exposure gate: when cfg.LAN.Exposed is false, LAN clients are denied
// even if they present valid Basic auth credentials. This catches the
// regression where stale credentials from a prior `lerd remote-control on`
// session would survive `lerd lan:unexpose` and silently allow LAN access.
func TestRemoteControlGate_lanOffOverridesCredentials(t *testing.T) {
	// Credentials are set, but lan.exposed is explicitly false.
	setupConfigDirRaw(t, "alice", "s3cret", false)

	next := &nextHandler{}
	gate := withRemoteControlGate(next)

	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	req.RemoteAddr = "192.168.1.42:54321"
	req.SetBasicAuth("alice", "s3cret")
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (LAN off should override valid creds)", rec.Code)
	}
	if next.called {
		t.Error("LAN client reached the handler despite lan.exposed=false")
	}
}

func TestRemoteControlGate_remoteSetupBypassesAuth(t *testing.T) {
	setupConfigDir(t, "alice", "s3cret") // even with auth set...

	next := &nextHandler{}
	gate := withRemoteControlGate(next)

	req := httptest.NewRequest(http.MethodGet, "/api/remote-setup?code=abc", nil)
	req.RemoteAddr = "192.168.1.42:54321" // ...and a LAN source IP
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if !next.called {
		t.Error("/api/remote-setup did not reach next handler")
	}
}

func TestRemoteControlGate_remoteSetupBypassesEvenWhenDisabled(t *testing.T) {
	setupConfigDir(t, "", "") // remote-control off

	next := &nextHandler{}
	gate := withRemoteControlGate(next)

	req := httptest.NewRequest(http.MethodGet, "/api/remote-setup?code=abc", nil)
	req.RemoteAddr = "192.168.1.42:54321"
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if !next.called {
		t.Error("/api/remote-setup blocked even though it has its own gate")
	}
}

func TestIsLoopbackOnlyPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/api/lerd/stop", true},
		{"/api/lerd/quit", true},
		{"/api/sites/link", true},
		{"/api/browse", true},
		{"/api/sites/myapp.test/terminal", true},
		{"/api/sites/foo.bar.test/terminal", true},
		{"/api/sites", false},
		{"/api/sites/myapp.test", false},
		{"/api/sites/myapp.test/secure", false},
		{"/api/lerd/start", false},
		{"/api/version", false},
		{"/", false},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			if got := isLoopbackOnlyPath(c.path); got != c.want {
				t.Errorf("isLoopbackOnlyPath(%q) = %v, want %v", c.path, got, c.want)
			}
		})
	}
}

func TestRemoteControlGate_loopbackOnlyRoutesBlockedFromLAN(t *testing.T) {
	setupConfigDir(t, "alice", "s3cret") // remote-control on with valid creds

	next := &nextHandler{}
	gate := withRemoteControlGate(next)

	cases := []string{
		"/api/lerd/stop",
		"/api/sites/link",
		"/api/sites/myapp.test/terminal",
		"/api/sites/myapp.test/env",
		"/api/browse",
		"/api/push/test",
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.RemoteAddr = "192.168.1.42:54321"
			req.SetBasicAuth("alice", "s3cret") // valid creds present
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403 (loopback-only path from LAN)", rec.Code)
			}
		})
	}
}

func TestRemoteControlGate_loopbackOnlyRoutesAllowedFromLoopback(t *testing.T) {
	setupConfigDir(t, "", "")

	next := &nextHandler{}
	gate := withRemoteControlGate(next)

	for _, path := range []string{"/api/lerd/stop", "/api/sites/link", "/api/sites/myapp.test/terminal"} {
		t.Run(path, func(t *testing.T) {
			next.called = false
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.RemoteAddr = "127.0.0.1:54321"
			// CSRF gate now blocks unauthenticated cross-origin POSTs
			// from loopback; the legitimate CLI caller sets X-Lerd-CSRF
			// and the dashboard JS sets Sec-Fetch-Site: same-origin. Pick
			// the header path here.
			req.Header.Set("X-Lerd-CSRF", "1")
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, req)
			if !next.called {
				t.Errorf("loopback request to %s blocked", path)
			}
		})
	}
}

func TestRemoteControlGate_optionsBypassesAuth(t *testing.T) {
	setupConfigDir(t, "alice", "s3cret")

	next := &nextHandler{}
	gate := withRemoteControlGate(next)

	req := httptest.NewRequest(http.MethodOptions, "/api/sites", nil)
	req.RemoteAddr = "192.168.1.42:54321" // LAN, no auth header
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if !next.called {
		t.Error("OPTIONS preflight blocked — CORS will fail")
	}
}

// Unix socket connections must be treated as loopback. The lerd.localhost
// nginx vhost reaches lerd-ui over the bind-mounted socket, and the request
// arrives with a non-IP RemoteAddr ("@"). Without the ctxKeyUnixSocket
// fast-path, the gate would 403 it the same as a LAN client and the
// dashboard would be unreachable via lerd.localhost. Regression test for
// the fix that replaced host.containers.internal:7073 with the unix socket.
func TestRemoteControlGate_unixSocketTreatedAsLoopback(t *testing.T) {
	setupConfigDirRaw(t, "", "", false) // LAN exposure off, no creds

	next := &nextHandler{}
	gate := withRemoteControlGate(next)

	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	req.RemoteAddr = "@" // typical for anonymous unix socket peer
	ctx := context.WithValue(req.Context(), ctxKeyUnixSocket{}, true)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if !next.called {
		t.Error("unix socket request blocked — lerd.localhost vhost will 403")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("unix socket status = %d, want 200", rec.Code)
	}
}

// The mailpit container POSTs to host.containers.internal:7073 and is
// source-NAT'd onto one of the host's interface IPs. The gate must let
// that through pre-auth (so fresh installs receive mail notifications)
// while still rejecting LAN attackers who arrive from a different IP.
func TestRemoteControlGate_mailpitWebhookHostAllowedLanBlocked(t *testing.T) {
	setupConfigDirRaw(t, "", "", false) // LAN off, no creds, default state

	addrs, err := net.InterfaceAddrs()
	if err != nil || len(addrs) == 0 {
		t.Skip("no interface addresses available")
	}
	var v4, v6 string
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
			continue
		}
		if ipNet.IP.To4() != nil && v4 == "" {
			v4 = ipNet.IP.String()
		} else if ipNet.IP.To4() == nil && v6 == "" {
			v6 = ipNet.IP.String()
		}
	}

	allowCases := []struct{ name, ip string }{}
	if v4 != "" {
		allowCases = append(allowCases, struct{ name, ip string }{"v4", v4})
		// A v6-only client that connects to a v4 listener arrives with the
		// v4-mapped form (::ffff:HOSTV4). fromHost relies on IP.Equal to
		// normalise across both shapes; pin it here so a future readers
		// don't reintroduce a string compare and break this path.
		allowCases = append(allowCases, struct{ name, ip string }{"v4_mapped_v6", "::ffff:" + v4})
	}
	if v6 != "" {
		allowCases = append(allowCases, struct{ name, ip string }{"v6", v6})
	}
	if len(allowCases) == 0 {
		t.Skip("no non-loopback host interfaces to probe")
	}
	for _, c := range allowCases {
		t.Run("allow_"+c.name, func(t *testing.T) {
			next := &nextHandler{}
			gate := withRemoteControlGate(next)
			req := httptest.NewRequest(http.MethodPost, "/api/webhooks/mailpit", nil)
			req.RemoteAddr = net.JoinHostPort(c.ip, "34567")
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, req)
			if !next.called {
				t.Errorf("mailpit webhook from host IP %s blocked, status=%d", c.ip, rec.Code)
			}
		})
	}

	denyCases := []struct{ name, ip string }{
		{"v4_lan", "198.51.100.42"},
		{"v6_documentation", "2001:db8::1"},
	}
	for _, c := range denyCases {
		t.Run("deny_"+c.name, func(t *testing.T) {
			next := &nextHandler{}
			gate := withRemoteControlGate(next)
			req := httptest.NewRequest(http.MethodPost, "/api/webhooks/mailpit", nil)
			req.RemoteAddr = net.JoinHostPort(c.ip, "34567")
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, req)
			if next.called {
				t.Errorf("mailpit webhook from non-host %s reached handler, want 403", c.ip)
			}
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
		})
	}
}

// fromHost must compare by IP value so an IPv6 source carrying a zone
// suffix (fe80::1%eth0) still matches the zoneless interface address.
func TestFromHost_acceptsZonedIPv6Source(t *testing.T) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skip("no interface addresses available")
	}
	var v6 string
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok || ipNet.IP == nil || ipNet.IP.To4() != nil {
			continue
		}
		v6 = ipNet.IP.String()
		break
	}
	if v6 == "" {
		t.Skip("no IPv6 interface address to probe")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/mailpit", nil)
	req.RemoteAddr = "[" + v6 + "%eth0]:34567"
	if !fromHost(req) {
		t.Errorf("fromHost rejected zoned IPv6 source %s%%eth0", v6)
	}
}

// TestRemoteControlGate_csrfBlocksCrossSitePost is the regression test
// for the CSRF→RCE primitive that let any browser-visited page POST to
// loopback endpoints like /api/sites/{d}/tinker (arbitrary PHP exec) by
// sending a no-cors simple-form request. The gate must reject any
// mutating method whose Sec-Fetch-Site says it came from a different
// origin, even when the source IP is loopback.
func TestRemoteControlGate_csrfBlocksCrossSitePost(t *testing.T) {
	setupConfigDirRaw(t, "", "", false)

	for _, site := range []string{"cross-site", "same-site"} {
		t.Run(site, func(t *testing.T) {
			next := &nextHandler{}
			gate := withRemoteControlGate(next)

			req := httptest.NewRequest(http.MethodPost, "/api/sites/my-app.test/tinker", nil)
			req.RemoteAddr = "127.0.0.1:54321"
			req.Header.Set("Sec-Fetch-Site", site)
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, req)

			if next.called {
				t.Errorf("Sec-Fetch-Site=%s reached handler — CSRF check failed open", site)
			}
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
		})
	}
}

// Sec-Fetch-Site: same-origin / none are the legitimate values the
// dashboard's own JS and the user's direct URL bar visits send. They
// must pass even on POST.
func TestRemoteControlGate_csrfAllowsSameOriginPost(t *testing.T) {
	setupConfigDirRaw(t, "", "", false)

	for _, site := range []string{"same-origin", "none"} {
		t.Run(site, func(t *testing.T) {
			next := &nextHandler{}
			gate := withRemoteControlGate(next)

			req := httptest.NewRequest(http.MethodPost, "/api/sites/my-app.test/tinker", nil)
			req.RemoteAddr = "127.0.0.1:54321"
			req.Header.Set("Sec-Fetch-Site", site)
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, req)

			if !next.called {
				t.Errorf("Sec-Fetch-Site=%s blocked — legitimate same-origin call rejected", site)
			}
		})
	}
}

// Curl and older browsers without Sec-Fetch-* must opt in explicitly
// via X-Lerd-CSRF. A POST with neither header gets a 403.
func TestRemoteControlGate_csrfRequiresHeaderWithoutSecFetch(t *testing.T) {
	setupConfigDirRaw(t, "", "", false)

	t.Run("no headers → 403", func(t *testing.T) {
		next := &nextHandler{}
		gate := withRemoteControlGate(next)

		req := httptest.NewRequest(http.MethodPost, "/api/sites/my-app.test/tinker", nil)
		req.RemoteAddr = "127.0.0.1:54321"
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)

		if next.called {
			t.Error("POST without Sec-Fetch-Site and without X-Lerd-CSRF reached handler")
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})

	t.Run("X-Lerd-CSRF set → passes", func(t *testing.T) {
		next := &nextHandler{}
		gate := withRemoteControlGate(next)

		req := httptest.NewRequest(http.MethodPost, "/api/sites/my-app.test/tinker", nil)
		req.RemoteAddr = "127.0.0.1:54321"
		req.Header.Set("X-Lerd-CSRF", "1")
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)

		if !next.called {
			t.Error("POST with X-Lerd-CSRF header blocked")
		}
	})
}

// GET / HEAD must pass the CSRF check unconditionally — they're
// non-mutating per RFC 9110 and the dashboard polls them constantly
// without setting any custom header. Note Sec-Fetch-Site: cross-site
// would still be rejected by CORS for any response the page wanted to
// read, but the gate itself doesn't block read-only methods.
func TestRemoteControlGate_csrfSkipsReadOnlyMethods(t *testing.T) {
	setupConfigDirRaw(t, "", "", false)

	for _, m := range []string{http.MethodGet, http.MethodHead} {
		t.Run(m, func(t *testing.T) {
			next := &nextHandler{}
			gate := withRemoteControlGate(next)

			req := httptest.NewRequest(m, "/api/sites", nil)
			req.RemoteAddr = "127.0.0.1:54321"
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, req)

			if !next.called {
				t.Errorf("%s with cross-site origin blocked by CSRF gate — should be read-only safe", m)
			}
		})
	}
}

// Unix-socket connections (the lerd.localhost nginx vhost) arrive with
// no Sec-Fetch-* headers and no CSRF token, but must pass — only host
// processes with filesystem access to the socket can reach this path.
func TestRemoteControlGate_csrfSkippedForUnixSocket(t *testing.T) {
	setupConfigDirRaw(t, "", "", false)

	next := &nextHandler{}
	gate := withRemoteControlGate(next)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/my-app.test/tinker", nil)
	req.RemoteAddr = "@"
	ctx := context.WithValue(req.Context(), ctxKeyUnixSocket{}, true)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if !next.called {
		t.Error("unix-socket POST blocked by CSRF gate — vhost will 403")
	}
}

// The CLI notifier (/api/internal/notify) is loopback-only and called
// without any browser context; it must skip CSRF or every CLI command
// would have to learn about the header.
func TestRemoteControlGate_csrfExemptInternalEndpoints(t *testing.T) {
	setupConfigDirRaw(t, "", "", false)

	exempt := []string{
		"/api/internal/notify",
		"/api/remote-setup",
		"/api/sites/my-app.test/unpause",
	}
	for _, path := range exempt {
		t.Run(path, func(t *testing.T) {
			next := &nextHandler{}
			gate := withRemoteControlGate(next)

			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.RemoteAddr = "127.0.0.1:54321"
			// Deliberately omit Sec-Fetch-Site and X-Lerd-CSRF — the path
			// is exempt either way.
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, req)

			if !next.called {
				t.Errorf("exempt path %s blocked by CSRF gate, status=%d", path, rec.Code)
			}
		})
	}
}

// LAN clients with valid Basic auth also need to pass CSRF on POST.
// The malicious-page-on-the-victim's-LAN scenario is real even when
// remote-control is enabled.
func TestRemoteControlGate_csrfEnforcedOnAuthenticatedLanPost(t *testing.T) {
	setupConfigDir(t, "alice", "s3cret")

	next := &nextHandler{}
	gate := withRemoteControlGate(next)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/my-app.test/tinker", nil)
	req.RemoteAddr = "192.168.1.42:54321"
	req.SetBasicAuth("alice", "s3cret")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if next.called {
		t.Error("authenticated LAN POST with cross-site origin reached handler — CSRF check missed LAN path")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// silence unused-import lint when config is only used transitively.
var _ = config.DataDir
