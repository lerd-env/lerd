package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// hostActionPaths covers one endpoint from each branch of isLoopbackOnlyPath:
// exact match, prefix subtree, and per-site subaction.
var hostActionPaths = []string{
	"/api/lerd/stop",
	"/api/logs/terminal",
	"/api/sites/link",
	"/api/browse",
	"/api/tools/composer/update",
	"/api/databases/mysql/drop",
	"/api/sites/myapp.test/env",
	"/api/sites/myapp.test/terminal",
}

// remoteRequest builds an authenticated request from a LAN address, carrying
// the CSRF header so the cross-origin gate isn't what rejects it.
func remoteRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "192.168.1.42:54321"
	req.Host = "dashboard.example.net"
	req.SetBasicAuth("alice", "s3cret")
	req.Header.Set("X-Lerd-CSRF", "1")
	return req
}

func TestHostActionRoutesBlockedFromRemoteByDefault(t *testing.T) {
	setupConfigDirFullAccess(t, "alice", "s3cret", false)

	for _, path := range hostActionPaths {
		t.Run(path, func(t *testing.T) {
			next := &nextHandler{}
			gate := withRemoteControlGate(next)
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, remoteRequest(http.MethodPost, path))

			if next.called {
				t.Error("host-action route reached the handler without remote full access")
			}
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
}

func TestHostActionRoutesAllowedFromRemoteWithFullAccess(t *testing.T) {
	setupConfigDirFullAccess(t, "alice", "s3cret", true)

	for _, path := range hostActionPaths {
		t.Run(path, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if !hasHostActionAuthority(r) {
					t.Error("handler-level authority denied a request the gate admitted")
				}
				w.WriteHeader(http.StatusOK)
			})
			rec := httptest.NewRecorder()
			withRemoteControlGate(next).ServeHTTP(rec, remoteRequest(http.MethodPost, path))

			if !called || rec.Code != http.StatusOK {
				t.Errorf("called=%v status=%d, want true/200", called, rec.Code)
			}
		})
	}
}

func TestHostActionRoutesAlwaysAllowedLocally(t *testing.T) {
	setupConfigDirFullAccess(t, "alice", "s3cret", false)

	for _, path := range hostActionPaths {
		t.Run(path, func(t *testing.T) {
			next := &nextHandler{}
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.RemoteAddr = "127.0.0.1:54321"
			req.Host = "localhost:7073"
			req.Header.Set("X-Lerd-CSRF", "1")
			rec := httptest.NewRecorder()
			withRemoteControlGate(next).ServeHTTP(rec, req)

			if !next.called {
				t.Errorf("local request to %s was blocked (status %d)", path, rec.Code)
			}
		})
	}
}

// Full access widens which routes an authenticated session may reach. It must
// never stand in for authentication itself.
func TestFullAccessStillRequiresCredentials(t *testing.T) {
	setupConfigDirFullAccess(t, "", "", true)

	next := &nextHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/browse", nil)
	req.RemoteAddr = "192.168.1.42:54321"
	req.Host = "dashboard.example.net"
	req.Header.Set("X-Lerd-CSRF", "1")
	rec := httptest.NewRecorder()
	withRemoteControlGate(next).ServeHTTP(rec, req)

	if next.called {
		t.Fatal("unauthenticated remote request reached a host-action route")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAccessModeReportsHostActionAuthority(t *testing.T) {
	for _, tc := range []struct {
		name       string
		fullAccess bool
		want       bool
	}{
		{"default", false, false},
		{"full access", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupConfigDirFullAccess(t, "alice", "s3cret", tc.fullAccess)

			rec := httptest.NewRecorder()
			gate := withRemoteControlGate(http.HandlerFunc(handleAccessMode))
			gate.ServeHTTP(rec, remoteRequest(http.MethodGet, "/api/access-mode"))

			var body struct {
				LocalControl bool `json:"local_control"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.LocalControl != tc.want {
				t.Errorf("local_control = %v, want %v", body.LocalControl, tc.want)
			}
		})
	}
}

// A remote session must not be able to widen its own authority.
func TestRemoteSessionCannotGrantItselfFullAccess(t *testing.T) {
	setupConfigDirFullAccess(t, "alice", "s3cret", false)

	req := httptest.NewRequest(http.MethodPost, "/api/remote-control",
		strings.NewReader(`{"action":"full-access","enabled":true}`))
	req.RemoteAddr = "192.168.1.42:54321"
	req.Host = "dashboard.example.net"
	req.SetBasicAuth("alice", "s3cret")
	req.Header.Set("X-Lerd-CSRF", "1")
	rec := httptest.NewRecorder()
	withRemoteControlGate(http.HandlerFunc(handleRemoteControl)).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.UI.RemoteFullAccess {
		t.Fatal("a remote session granted itself full access")
	}
}

func TestLocalRequestCanToggleFullAccess(t *testing.T) {
	setupConfigDirFullAccess(t, "alice", "s3cret", false)

	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"action":"full-access","enabled":true}`, true},
		{`{"action":"full-access","enabled":false}`, false},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/remote-control", strings.NewReader(tc.body))
		req.RemoteAddr = "127.0.0.1:54321"
		req.Host = "localhost:7073"
		req.Header.Set("X-Lerd-CSRF", "1")
		rec := httptest.NewRecorder()
		withRemoteControlGate(http.HandlerFunc(handleRemoteControl)).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		cfg, err := config.LoadGlobal()
		if err != nil {
			t.Fatalf("LoadGlobal: %v", err)
		}
		if cfg.UI.RemoteFullAccess != tc.want {
			t.Fatalf("ui.remote_full_access = %v, want %v", cfg.UI.RemoteFullAccess, tc.want)
		}
	}
}

func TestIsLoopbackOnlyPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/api/lerd/stop", true},
		{"/api/lerd/quit", true},
		{"/api/logs/terminal", true},
		{"/api/logs/lerd-nginx", false},
		{"/api/sites/link", true},
		{"/api/browse", true},
		{"/api/sites/myapp.test/terminal", true},
		{"/api/sites/foo.bar.test/terminal", true},
		{"/api/sites/myapp.test/env", true},
		{"/api/sites/myapp.test/env/files", true},
		{"/api/sites/myapp.test/env/backups", true},
		{"/api/sites/myapp.test/env/backups/.env.bkp.20260528-103045", true},
		{"/api/sites/myapp.test/env/restore", true},
		{"/api/sites/myapp.test/terminal/anything", true},
		{"/api/databases", true},
		{"/api/databases/mysql", true},
		{"/api/databases/mysql/drop", true},
		{"/api/databases/mysql/export", true},
		{"/api/databases/postgres/snapshots/nightly", true},
		{"/api/databases-overview", false},
		{"/api/tools/composer/update", true},
		{"/api/share-tools", false},
		{"/api/sites", false},
		{"/api/sites/myapp.test", false},
		{"/api/sites/myapp.test/secure", false},
		{"/api/sites/myapp.test/envoy", false},
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
