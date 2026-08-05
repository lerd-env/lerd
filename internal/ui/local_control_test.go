package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A browser on the lerd host may reach the dashboard by a name that is not
// "localhost": Debian and Ubuntu map the machine's hostname to 127.0.1.1, and
// /etc/hosts aliases are common. Those requests must keep working, or the local
// user is locked out of their own dashboard with no credential that helps.
func TestLocalControlAcceptsLoopbackHostnames(t *testing.T) {
	setupConfigDirRaw(t, "", "", false) // LAN off, no credentials: only local works

	for _, tc := range []struct{ name, peer, host string }{
		{"localhost", "127.0.0.1:54321", "localhost:7073"},
		{"loopback ip", "127.0.0.1:54321", "127.0.0.1:7073"},
		{"nginx vhost", "127.0.0.1:54321", "lerd.localhost"},
		{"machine hostname", "127.0.1.1:54321", "workstation:7073"},
		{"hosts alias", "127.0.0.1:54321", "dev.internal"},
		{"ipv6 loopback", "[::1]:54321", "[::1]:7073"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := &nextHandler{}
			req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
			req.RemoteAddr = tc.peer
			req.Host = tc.host
			rec := httptest.NewRecorder()
			withRemoteControlGate(next).ServeHTTP(rec, req)

			if !next.called {
				t.Errorf("local request with Host %q was blocked (status %d)", tc.host, rec.Code)
			}
		})
	}
}

// A reverse proxy relaying a remote browser also connects from 127.0.0.1. The
// forwarding headers it adds are what separates it from a local browser.
func TestLocalControlRejectsForwardedRequests(t *testing.T) {
	for _, header := range proxyHeaders {
		t.Run(header, func(t *testing.T) {
			setupConfigDirRaw(t, "", "", true) // LAN exposed, no credentials

			next := &nextHandler{}
			req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
			req.RemoteAddr = "127.0.0.1:54321"
			req.Host = "dashboard.example.ts.net"
			req.Header.Set(header, "203.0.113.7")
			rec := httptest.NewRecorder()
			withRemoteControlGate(next).ServeHTTP(rec, req)

			if next.called {
				t.Errorf("%s did not stop a proxied request from bypassing authentication", header)
			}
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
}

// Authentication lets a proxied remote session reach handlers that are not on
// the loopback-only list. Those handlers still guard themselves, and that guard
// must apply the same forwarding test the middleware does: a proxy relaying a
// remote browser connects from 127.0.0.1, so a peer-only check hands it host
// authority the full-access opt-in was meant to withhold.
func TestHostActionAuthorityRejectsProxiedRemoteWithoutFullAccess(t *testing.T) {
	setupConfigDirFullAccess(t, "alice", "s3cret", false)

	var authority bool
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		authority = hasHostActionAuthority(r)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/open-editor", nil)
	req.RemoteAddr = "127.0.0.1:54321" // the reverse proxy, on the host itself
	req.Host = "dashboard.example.ts.net"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.Header.Set("X-Lerd-CSRF", "1")
	req.SetBasicAuth("alice", "s3cret")
	withRemoteControlGate(next).ServeHTTP(httptest.NewRecorder(), req)

	if authority {
		t.Error("a proxied remote session was granted host-action authority with full access off")
	}
}

// The same session gets that authority once the user opts in, or the opt-in
// would do nothing for the setup it exists to serve.
func TestHostActionAuthorityAllowsProxiedRemoteWithFullAccess(t *testing.T) {
	setupConfigDirFullAccess(t, "alice", "s3cret", true)

	var authority bool
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		authority = hasHostActionAuthority(r)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/open-editor", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "dashboard.example.ts.net"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.Header.Set("X-Lerd-CSRF", "1")
	req.SetBasicAuth("alice", "s3cret")
	withRemoteControlGate(next).ServeHTTP(httptest.NewRecorder(), req)

	if !authority {
		t.Error("full access did not reach a proxied remote session")
	}
}

// The unix socket carries no peer address and no forwarding headers; it is
// reached only by host processes, so it stays authoritative.
func TestLocalControlAcceptsUnixSocketWithForeignHost(t *testing.T) {
	setupConfigDirRaw(t, "", "", false)

	next := &nextHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	req.RemoteAddr = ""
	req.Host = "lerd.localhost"
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyUnixSocket{}, true))
	rec := httptest.NewRecorder()
	withRemoteControlGate(next).ServeHTTP(rec, req)

	if !next.called {
		t.Fatalf("unix-socket request was blocked (status %d)", rec.Code)
	}
}
