package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/pfredirect"
	"github.com/geodro/lerd/internal/platform"
	"github.com/geodro/lerd/internal/portforward"
)

// claimChain is the part of lerd-ui's handler a lerd.localhost request crosses.
func claimChain(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(vhostClaimPath, handleVhostClaim)
	mux.Handle("/", next)
	return withVhostClaim(withDashboardMounts(withRemoteControlGate(mux)))
}

// viaVhost is a request as nginx relays it from the lerd.localhost vhost: over
// the unix socket, with the forwarding headers the vhost sets.
func viaVhost(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = ""
	req.Host = "lerd.localhost"
	req.Header.Set("X-Real-IP", "10.89.0.5")
	req.Header.Set("X-Forwarded-For", "10.89.0.5")
	return req.WithContext(context.WithValue(req.Context(), ctxKeyUnixSocket{}, true))
}

// With LAN exposure on, nginx sees every client as the same container address,
// so a LAN device sending Host: lerd.localhost got the dashboard and /_svc/
// with local authority while :7073 asked it for the password.
func TestVhostWithoutClaimIsNotLocalWhileLANExposed(t *testing.T) {
	setupConfigDirRaw(t, "", "", true)
	next := &nextHandler{}
	rec := httptest.NewRecorder()
	claimChain(next).ServeHTTP(rec, viaVhost(http.MethodGet, "/_svc/mailpit/api/v1/messages"))

	if next.called {
		t.Fatal("an unclaimed vhost request reached the dashboard while LAN is exposed")
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want a bounce through the loopback port", rec.Code)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc.Host != "127.0.0.1:7073" || loc.Path != vhostClaimPath || loc.Query().Get("next") != "/_svc/mailpit/api/v1/messages" {
		t.Errorf("Location = %q", rec.Header().Get("Location"))
	}
	if isLocalControlRequest(viaVhost(http.MethodGet, "/")) {
		t.Error("isLocalControlRequest trusted an unclaimed vhost request while LAN is exposed")
	}
}

func TestVhostUnsafeMethodWithoutClaimIsRefused(t *testing.T) {
	setupConfigDirRaw(t, "", "", true)
	next := &nextHandler{}
	req := viaVhost(http.MethodPost, "/_svc/mailpit/api/v1/messages")
	req.Header.Set("X-Lerd-CSRF", "1")
	rec := httptest.NewRecorder()
	claimChain(next).ServeHTTP(rec, req)

	if next.called || rec.Code != http.StatusForbidden {
		t.Fatalf("called = %v, status = %d, want 403", next.called, rec.Code)
	}
}

func TestVhostStaysLocalWhileLANIsNotExposed(t *testing.T) {
	setupConfigDirRaw(t, "", "", false)
	next := &nextHandler{}
	rec := httptest.NewRecorder()
	claimChain(next).ServeHTTP(rec, viaVhost(http.MethodGet, "/_svc/mailpit/"))

	if !next.called {
		t.Fatalf("vhost request blocked with LAN exposure off (status %d)", rec.Code)
	}
}

// The local browser proves itself once on 127.0.0.1:7073, which only the host
// reaches, and keeps working on lerd.localhost from then on.
func TestVhostClaimRoundTripKeepsLerdLocalhostWorking(t *testing.T) {
	setupConfigDirRaw(t, "", "", true)
	next := &nextHandler{}
	h := claimChain(next)

	issue := httptest.NewRequest(http.MethodGet, vhostClaimPath+"?next=/_svc/mailpit/", nil)
	issue.RemoteAddr = "127.0.0.1:50000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, issue)
	back, _ := url.Parse(rec.Header().Get("Location"))
	if rec.Code != http.StatusFound || back.Host != "lerd.localhost" {
		t.Fatalf("issue: status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, viaVhost(http.MethodGet, back.RequestURI()))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/_svc/mailpit/" {
		t.Fatalf("redeem: status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatalf("redeem set cookies %+v, want one HttpOnly cookie", cookies)
	}

	req := viaVhost(http.MethodGet, "/_svc/mailpit/")
	req.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !next.called {
		t.Fatalf("claimed vhost request blocked (status %d)", rec.Code)
	}
	if !isLocalControlRequest(req) {
		t.Error("a claimed vhost request lost local authority")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, viaVhost(http.MethodGet, back.RequestURI()))
	if len(rec.Result().Cookies()) != 0 {
		t.Error("a claim nonce was accepted twice")
	}
}

func TestVhostClaimIsNotIssuedToALANPeer(t *testing.T) {
	setupConfigDirRaw(t, "alice", "s3cret", true)
	issue := httptest.NewRequest(http.MethodGet, vhostClaimPath+"?next=/", nil)
	issue.RemoteAddr = "192.168.1.50:50000"
	issue.SetBasicAuth("alice", "s3cret")
	rec := httptest.NewRecorder()
	claimChain(&nextHandler{}).ServeHTTP(rec, issue)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a claim from the LAN", rec.Code)
	}
}

func TestVhostClaimForgedCookieIsRejected(t *testing.T) {
	setupConfigDirRaw(t, "", "", true)
	req := viaVhost(http.MethodGet, "/_svc/mailpit/")
	req.AddCookie(&http.Cookie{Name: vhostClaimCookie, Value: "guess"})
	if isLocalControlRequest(req) {
		t.Error("a made-up claim cookie was trusted")
	}
}

func TestVhostClaimNextStaysOnTheVhost(t *testing.T) {
	for in, want := range map[string]string{
		"/_svc/x/?a=1":      "/_svc/x/?a=1",
		"//evil.example/":   "/",
		"https://evil.test": "/",
		"":                  "/",
		"/\\evil.example":   "/",
		"/\t/evil.example":  "/",
		"/\n/evil.example":  "/",
		"/\r/evil.example":  "/",
	} {
		if got := safeClaimNext(in); got != want {
			t.Errorf("safeClaimNext(%q) = %q, want %q", in, got, want)
		}
	}
}

// A browser follows the Location http.Redirect writes, which path-cleans what
// safeClaimNext let through, so the header itself is what must stay local.
func TestVhostClaimRedirectStaysOnTheVhost(t *testing.T) {
	setupConfigDirRaw(t, "", "", true)
	for next, want := range map[string]string{
		"/./\\evil.example/":        "/",
		"/a/../\\evil.example/":     "/",
		"/a#/../\\evil.example/":    "/",
		"/./\\evil.example/?x=1":    "/",
		"/_svc/x/?c=App\\Models\\A": "/_svc/x/?c=App\\Models\\A",
	} {
		target := "/?" + vhostClaimParam + "=" + vhostClaims.issue() + "&next=" + url.QueryEscape(next)
		rec := httptest.NewRecorder()
		claimChain(&nextHandler{}).ServeHTTP(rec, viaVhost(http.MethodGet, target))
		if got := rec.Header().Get("Location"); rec.Code != http.StatusFound || got != want {
			t.Errorf("next=%q: code %d, Location %q, want %q", next, rec.Code, got, want)
		}
	}
}

// viaMachineVhost is the same relay on macOS: nginx runs in the podman VM and
// reaches lerd-ui over TCP, which gvproxy hands over from a host address, with
// the trust token instead of the unix socket.
func viaMachineVhost(t *testing.T, method, target string) *http.Request {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	token, err := nginx.LoadOrGenerateTrustToken()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = "127.0.0.1:50000"
	req.Host = "lerd.localhost"
	req.Header.Set("X-Real-IP", "192.168.127.1")
	req.Header.Set("X-Forwarded-For", "192.168.127.1")
	req.Header.Set("X-Lerd-Trust", token)
	return req
}

// The macOS vhost carries the trust token, which was accepted before the claim
// was looked at, so LAN clients kept local authority there.
func TestMachineVhostWithoutClaimIsNotLocalWhileLANExposed(t *testing.T) {
	setupConfigDirRaw(t, "", "", true)
	next := &nextHandler{}
	rec := httptest.NewRecorder()
	claimChain(next).ServeHTTP(rec, viaMachineVhost(t, http.MethodGet, "/_svc/mailpit/"))

	if next.called || rec.Code != http.StatusFound {
		t.Fatalf("called = %v, status = %d, want a bounce through the loopback port", next.called, rec.Code)
	}
	if isLocalControlRequest(viaMachineVhost(t, http.MethodGet, "/")) {
		t.Error("isLocalControlRequest trusted an unclaimed macOS vhost request")
	}
	if isLoopbackRequest(viaMachineVhost(t, http.MethodGet, "/")) {
		t.Error("isLoopbackRequest trusted an unclaimed macOS vhost request")
	}
}

func TestMachineVhostClaimIsNotIssuedThroughTheVhost(t *testing.T) {
	setupConfigDirRaw(t, "", "", true)
	rec := httptest.NewRecorder()
	claimChain(&nextHandler{}).ServeHTTP(rec, viaMachineVhost(t, http.MethodGet, vhostClaimPath+"?next=/"))
	if loc, _ := url.Parse(rec.Header().Get("Location")); loc != nil && loc.Query().Get(vhostClaimParam) != "" {
		t.Fatalf("a claim nonce was issued through the vhost: %s", loc)
	}

	// Even with the bounce out of the way, the handler itself must refuse.
	rec = httptest.NewRecorder()
	handleVhostClaim(rec, viaMachineVhost(t, http.MethodGet, vhostClaimPath+"?next=/"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a claim asked through the vhost", rec.Code)
	}
}

func TestMachineVhostWithClaimStaysLocal(t *testing.T) {
	setupConfigDirRaw(t, "", "", true)
	req := viaMachineVhost(t, http.MethodGet, "/_svc/mailpit/")
	req.AddCookie(&http.Cookie{Name: vhostClaimCookie, Value: vhostClaims.secret})
	if !isLocalControlRequest(req) || !isLoopbackRequest(req) {
		t.Error("a claimed macOS vhost request lost local authority")
	}
}

func TestMachineVhostStaysLocalWhileLANIsNotExposed(t *testing.T) {
	setupConfigDirRaw(t, "", "", false)
	next := &nextHandler{}
	claimChain(next).ServeHTTP(httptest.NewRecorder(), viaMachineVhost(t, http.MethodGet, "/_svc/mailpit/"))
	if !next.called {
		t.Fatal("macOS vhost request blocked with LAN exposure off")
	}
}

// nginxPublishedWide puts the test on macOS with lerd-nginx bound to loopback
// 80, and a pf redirect for it only when redirected is true.
func nginxPublishedWide(t *testing.T, redirected bool) {
	t.Helper()
	prev := platform.Current
	platform.Current.PrivilegedPortsBindAll = true
	t.Cleanup(func() { platform.Current = prev })
	if err := os.MkdirAll(config.QuadletDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	unit := "[Container]\nPublishPort=127.0.0.1:80:80\nPublishPort=127.0.0.1:443:443\n"
	if err := os.WriteFile(filepath.Join(config.QuadletDir(), "lerd-nginx.container"), []byte(unit), 0o644); err != nil {
		t.Fatal(err)
	}
	anchor := filepath.Join(t.TempDir(), "lerd")
	if redirected {
		if err := os.WriteFile(anchor, []byte(pfredirect.Render(pfredirect.Ports())), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prevAnchor, prevDir := pfredirect.AnchorPath, portforward.PlistDir
	pfredirect.AnchorPath, portforward.PlistDir = anchor, t.TempDir()
	t.Cleanup(func() { pfredirect.AnchorPath, portforward.PlistDir = prevAnchor, prevDir })
}

// Without the pf redirect, macOS publishes 80 and 443 on every interface even
// with LAN exposure off, so the vhost cannot vouch for the browser there either.
func TestMachineVhostNeedsClaimWhenNginxIsPublishedWide(t *testing.T) {
	setupConfigDirRaw(t, "", "", false)
	nginxPublishedWide(t, false)
	next := &nextHandler{}
	rec := httptest.NewRecorder()
	claimChain(next).ServeHTTP(rec, viaMachineVhost(t, http.MethodGet, "/_svc/mailpit/"))

	if next.called || rec.Code != http.StatusFound {
		t.Fatalf("called = %v, status = %d, want a bounce while 80 and 443 reach the network", next.called, rec.Code)
	}
}

func TestMachineVhostStaysLocalBehindThePfRedirect(t *testing.T) {
	setupConfigDirRaw(t, "", "", false)
	nginxPublishedWide(t, true)
	next := &nextHandler{}
	claimChain(next).ServeHTTP(httptest.NewRecorder(), viaMachineVhost(t, http.MethodGet, "/_svc/mailpit/"))
	if !next.called {
		t.Fatal("a vhost request was bounced although the pf redirect keeps nginx on loopback")
	}
}
