package ui

import (
	"context"
	"encoding/json"
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
	setupConfigDirWith(t, username, plainPassword, lanExposed, false)
}

// setupConfigDirFullAccess is setupConfigDir with ui.remote_full_access set,
// for the tests that exercise the host-action opt-in. LAN exposure is on so
// the remote requests reach the authentication step.
func setupConfigDirFullAccess(t *testing.T, username, plainPassword string, fullAccess bool) {
	t.Helper()
	setupConfigDirWith(t, username, plainPassword, true, fullAccess)
}

func setupConfigDirWith(t *testing.T, username, plainPassword string, lanExposed, fullAccess bool) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg := map[string]any{}
	if lanExposed {
		cfg["lan"] = map[string]any{"exposed": true}
	}
	ui := map[string]any{}
	if username != "" || plainPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.MinCost)
		if err != nil {
			t.Fatalf("bcrypt: %v", err)
		}
		ui["username"] = username
		ui["password_hash"] = string(hash)
	}
	if fullAccess {
		ui["remote_full_access"] = true
	}
	if len(ui) > 0 {
		cfg["ui"] = ui
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
	req.Host = "localhost:7073"
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if !next.called {
		t.Error("loopback request did not reach next handler")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("loopback status = %d, want 200", rec.Code)
	}
}

func TestRemoteControlGateReverseProxyDoesNotBypassAuthentication(t *testing.T) {
	setupConfigDirRaw(t, "", "", true)
	next := &nextHandler{}
	gate := withRemoteControlGate(next)

	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "robotbox.example.net"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if next.called {
		t.Fatal("loopback reverse proxy bypassed dashboard authentication")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRemoteControlGateAuthenticatedReverseProxyReceivesDashboardControl(t *testing.T) {
	setupConfigDirFullAccess(t, "alice", "s3cret", true)
	gate := withRemoteControlGate(http.HandlerFunc(handleAccessMode))

	req := httptest.NewRequest(http.MethodGet, "/api/access-mode", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "robotbox.example.net"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.SetBasicAuth("alice", "s3cret")
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	var body struct {
		LocalControl bool `json:"local_control"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.LocalControl {
		t.Fatalf("response = %+v, authenticated dashboard must receive full controls", body)
	}
}

func TestRemoteControlGateAuthenticatedDashboardCanMutateLANSettings(t *testing.T) {
	setupConfigDirFullAccess(t, "alice", "s3cret", true)
	gate := withRemoteControlGate(http.HandlerFunc(handleLANStatus))

	req := httptest.NewRequest(http.MethodPost, "/api/lan/status", http.NoBody)
	req.RemoteAddr = "192.168.1.42:54321"
	req.Host = "robotbox.example.net"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.SetBasicAuth("alice", "s3cret")
	req.Header.Set("X-Lerd-CSRF", "1")
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 after authenticated request reaches action validation", rec.Code)
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

// TestRemoteControlGate_sessionCookie verifies that a successful Basic auth
// mints a session cookie and that the cookie alone authenticates a later LAN
// request without re-presenting the Authorization header. A tampered cookie
// falls through to the 401 challenge. This is the iOS Safari fix: Safari drops
// cached Basic credentials between refreshes, so the cookie carries the
// session instead.
func TestRemoteControlGate_sessionCookie(t *testing.T) {
	setupConfigDir(t, "alice", "s3cret")

	// Basic auth success must set the session cookie.
	next := &nextHandler{}
	gate := withRemoteControlGate(next)
	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	req.RemoteAddr = "192.168.1.42:54321"
	req.SetBasicAuth("alice", "s3cret")
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Basic auth status = %d, want 200", rec.Code)
	}
	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == remoteSessionCookie {
			session = c
		}
	}
	if session == nil {
		t.Fatal("no session cookie set after successful Basic auth")
	}

	t.Run("cookie authenticates without Basic header", func(t *testing.T) {
		next2 := &nextHandler{}
		gate2 := withRemoteControlGate(next2)
		req2 := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
		req2.RemoteAddr = "192.168.1.42:54321"
		req2.AddCookie(session)
		rec2 := httptest.NewRecorder()
		gate2.ServeHTTP(rec2, req2)
		if !next2.called || rec2.Code != http.StatusOK {
			t.Errorf("session cookie did not authenticate, status=%d", rec2.Code)
		}
	})

	t.Run("tampered cookie falls through to 401", func(t *testing.T) {
		flip := byte('0')
		if session.Value[len(session.Value)-1] == '0' {
			flip = '1'
		}
		next2 := &nextHandler{}
		gate2 := withRemoteControlGate(next2)
		req2 := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
		req2.RemoteAddr = "192.168.1.42:54321"
		req2.AddCookie(&http.Cookie{Name: remoteSessionCookie, Value: session.Value[:len(session.Value)-1] + string(flip)})
		rec2 := httptest.NewRecorder()
		gate2.ServeHTTP(rec2, req2)
		if next2.called || rec2.Code != http.StatusUnauthorized {
			t.Errorf("tampered cookie status = %d, want 401", rec2.Code)
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

// Ordinary dashboard routes are the ones a remote session is meant to drive.
// The host-action subset is covered in remote_full_access_test.go.
func TestRemoteControlGateAuthenticatedDashboardUsesOrdinaryRoutes(t *testing.T) {
	setupConfigDir(t, "alice", "s3cret")
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	gate := withRemoteControlGate(next)

	for _, path := range []string{
		"/api/lerd/start",
		"/api/sites/myapp.test/secure",
		"/api/sites/myapp.test/restart",
		"/api/services/mysql/restart",
		"/api/dumps/toggle",
	} {
		t.Run(path, func(t *testing.T) {
			called = false
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.RemoteAddr = "192.168.1.42:54321"
			req.Host = "robotbox.example.net"
			req.Header.Set("X-Forwarded-For", "203.0.113.7")
			req.Header.Set("X-Forwarded-For", "203.0.113.7")
			req.SetBasicAuth("alice", "s3cret")
			req.Header.Set("X-Lerd-CSRF", "1")
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, req)

			if !called || rec.Code != http.StatusOK {
				t.Fatalf("authenticated dashboard route %s: called=%v status=%d", path, called, rec.Code)
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

// TestRemoteControlGate_csrf covers the cross-origin gate that guards every
// state-changing request, loopback included. The RCE vector is a malicious
// page in the developer's own browser POSTing to 127.0.0.1:7073, so a
// loopback source IP is no longer a free pass for unsafe methods: the request
// must also prove it came from lerd's own dashboard.
func TestRemoteControlGate_csrf(t *testing.T) {
	const tinker = "/api/sites/myapp.test/tinker"

	t.Run("cross-site POST blocked", func(t *testing.T) {
		next := &nextHandler{}
		gate := withRemoteControlGate(next)
		req := httptest.NewRequest(http.MethodPost, tinker, nil)
		req.RemoteAddr = "127.0.0.1:54321"
		req.Host = "localhost:7073"
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set("Origin", "http://evil.example")
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		if next.called {
			t.Error("cross-site POST reached handler — CSRF/RCE vector open")
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})

	t.Run("same-origin POST allowed", func(t *testing.T) {
		next := &nextHandler{}
		gate := withRemoteControlGate(next)
		req := httptest.NewRequest(http.MethodPost, tinker, nil)
		req.RemoteAddr = "127.0.0.1:54321"
		req.Host = "localhost:7073"
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		if !next.called {
			t.Errorf("same-origin POST blocked, status=%d", rec.Code)
		}
	})

	// lerd.localhost hitting localhost:7073 (apiBase rewrite) is labelled
	// cross-site by the browser, but the Origin is one of lerd's own, so the
	// dashboard's own requests must still pass.
	t.Run("split-origin dashboard allowed via Origin allowlist", func(t *testing.T) {
		next := &nextHandler{}
		gate := withRemoteControlGate(next)
		req := httptest.NewRequest(http.MethodPost, tinker, nil)
		req.RemoteAddr = "127.0.0.1:54321"
		req.Host = "localhost:7073"
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set("Origin", "http://lerd.localhost")
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		if !next.called {
			t.Errorf("split-origin dashboard POST blocked, status=%d", rec.Code)
		}
	})

	t.Run("no Sec-Fetch requires CSRF header", func(t *testing.T) {
		next := &nextHandler{}
		gate := withRemoteControlGate(next)
		req := httptest.NewRequest(http.MethodPost, tinker, nil)
		req.RemoteAddr = "127.0.0.1:54321" // no Sec-Fetch, no X-Lerd-CSRF
		req.Host = "localhost:7073"
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		if next.called || rec.Code != http.StatusForbidden {
			t.Errorf("POST without Sec-Fetch or CSRF header allowed, status=%d", rec.Code)
		}

		next2 := &nextHandler{}
		gate2 := withRemoteControlGate(next2)
		req2 := httptest.NewRequest(http.MethodPost, tinker, nil)
		req2.RemoteAddr = "127.0.0.1:54321"
		req2.Host = "localhost:7073"
		req2.Header.Set("X-Lerd-CSRF", "1")
		rec2 := httptest.NewRecorder()
		gate2.ServeHTTP(rec2, req2)
		if !next2.called {
			t.Errorf("POST with X-Lerd-CSRF blocked, status=%d", rec2.Code)
		}
	})

	t.Run("safe methods bypass the gate", func(t *testing.T) {
		for _, m := range []string{http.MethodGet, http.MethodHead} {
			next := &nextHandler{}
			gate := withRemoteControlGate(next)
			req := httptest.NewRequest(m, "/api/sites", nil)
			req.RemoteAddr = "127.0.0.1:54321"
			req.Host = "localhost:7073"
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			req.Header.Set("Origin", "http://evil.example")
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, req)
			if !next.called {
				t.Errorf("%s blocked by CSRF gate, status=%d", m, rec.Code)
			}
		}
	})

	t.Run("unix socket exempt", func(t *testing.T) {
		next := &nextHandler{}
		gate := withRemoteControlGate(next)
		req := httptest.NewRequest(http.MethodPost, tinker, nil)
		req.RemoteAddr = "@" // no Sec-Fetch, no header — trusted via the socket
		req = req.WithContext(context.WithValue(req.Context(), ctxKeyUnixSocket{}, true))
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		if !next.called {
			t.Errorf("unix socket POST blocked, status=%d", rec.Code)
		}
	})

	// These endpoints are reached by non-browser clients (or cross-origin
	// pages we can't control) that can't carry the header, and each keeps its
	// own source gate: unpause from the paused-site holding page, the internal
	// notify bridge POSTed by out-of-process CLI/MCP commands over loopback.
	t.Run("exempt paths bypass the gate", func(t *testing.T) {
		for _, path := range []string{"/api/sites/myapp.test/unpause", "/api/internal/notify"} {
			t.Run(path, func(t *testing.T) {
				next := &nextHandler{}
				gate := withRemoteControlGate(next)
				req := httptest.NewRequest(http.MethodPost, path, nil)
				req.RemoteAddr = "127.0.0.1:54321" // no Sec-Fetch, no X-Lerd-CSRF
				req.Host = "localhost:7073"
				rec := httptest.NewRecorder()
				gate.ServeHTTP(rec, req)
				if !next.called {
					t.Errorf("%s blocked by CSRF gate, status=%d", path, rec.Code)
				}
			})
		}
	})

	t.Run("LAN cross-site rejected even with valid auth", func(t *testing.T) {
		setupConfigDir(t, "alice", "s3cret")
		next := &nextHandler{}
		gate := withRemoteControlGate(next)
		req := httptest.NewRequest(http.MethodPost, tinker, nil)
		req.RemoteAddr = "192.168.1.42:54321"
		req.SetBasicAuth("alice", "s3cret")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set("Origin", "http://evil.example")
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		if next.called || rec.Code != http.StatusForbidden {
			t.Errorf("LAN cross-site POST allowed, status=%d", rec.Code)
		}
	})
}

// silence unused-import lint when config is only used transitively.
var _ = config.DataDir
