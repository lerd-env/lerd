package ui

import (
	"crypto/rand"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dashboard"
	"github.com/geodro/lerd/internal/podman"
)

// With LAN exposure on, nginx relays every client from one container address,
// so a lerd.localhost request proves nothing. The local browser proves itself
// once on 127.0.0.1:7073, which only the host reaches, and keeps a cookie.

const (
	vhostClaimPath   = "/_lerd/claim"
	vhostClaimCookie = "lerd_local"
	vhostClaimParam  = "lerd-claim"
	claimNonceTTL    = time.Minute
)

var vhostClaims = &claimStore{secret: rand.Text(), nonces: map[string]time.Time{}}

// claimStore holds the cookie value, fresh for each lerd-ui run, and the
// one-time nonces that carry a claim from :7073 to the vhost.
type claimStore struct {
	mu     sync.Mutex
	secret string
	nonces map[string]time.Time
}

func (s *claimStore) issue() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for n, exp := range s.nonces {
		if now.After(exp) {
			delete(s.nonces, n)
		}
	}
	nonce := rand.Text()
	s.nonces[nonce] = now.Add(claimNonceTTL)
	return nonce
}

func (s *claimStore) redeem(nonce string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.nonces[nonce]
	delete(s.nonces, nonce)
	return ok && time.Now().Before(exp)
}

// relayedByVhost reports whether r came through the lerd.localhost vhost: over
// the unix socket on Linux, or with the trust token from the macOS VM.
func relayedByVhost(r *http.Request) bool {
	if v, _ := r.Context().Value(ctxKeyUnixSocket{}).(bool); v {
		return true
	}
	return hasValidTrustToken(r)
}

// needsVhostClaim reports whether r came through nginx while port 80 is open
// to the LAN: exposed on purpose, or published wide on macOS without the pf
// redirect. An unreadable config or unit counts as open, so it fails closed.
func needsVhostClaim(r *http.Request) bool {
	if !relayedByVhost(r) || !forwardedByProxy(r) {
		return false
	}
	if cfg, err := config.LoadGlobal(); err != nil || cfg == nil || cfg.LAN.Exposed {
		return true
	}
	wide, err := podman.UnitLoopbackPortsPublishedWide("lerd-nginx")
	return err != nil || len(wide) > 0
}

func hasVhostClaim(r *http.Request) bool {
	c, err := r.Cookie(vhostClaimCookie)
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(vhostClaims.secret)) == 1
}

// vhostIsLocal is the authority of the vhost relay: every host process that
// uses it, except a LAN-exposed request that has not proven a local browser.
func vhostIsLocal(r *http.Request) bool {
	return !needsVhostClaim(r) || hasVhostClaim(r)
}

// withVhostClaim bounces an unproven vhost request through :7073 and redeems
// the claim it comes back with. Anything else passes through untouched.
func withVhostClaim(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !needsVhostClaim(r) || hasVhostClaim(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if nonce := r.URL.Query().Get(vhostClaimParam); r.URL.Path == "/" && nonce != "" {
			redeemVhostClaim(w, r, nonce)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Forbidden — open the dashboard on this machine first.", http.StatusForbidden)
			return
		}
		http.Redirect(w, r, dashboard.DirectURL+vhostClaimPath+"?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
	})
}

func redeemVhostClaim(w http.ResponseWriter, r *http.Request, nonce string) {
	if !vhostClaims.redeem(nonce) {
		http.Error(w, "Forbidden — this claim has expired, reload the page.", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     vhostClaimCookie,
		Value:    vhostClaims.secret,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, safeClaimNext(r.URL.Query().Get("next")), http.StatusFound)
}

// handleVhostClaim issues a claim on :7073. Only a direct loopback request
// qualifies: the vhost relay is the very path the claim is meant to vouch for.
func handleVhostClaim(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if relayedByVhost(r) || !isLocalControlRequest(r) {
		http.Error(w, "Forbidden — a dashboard claim is only issued on this machine.", http.StatusForbidden)
		return
	}
	next := safeClaimNext(r.URL.Query().Get("next"))
	http.Redirect(w, r, dashboard.VhostURL+"/?"+vhostClaimParam+"="+vhostClaims.issue()+"&next="+url.QueryEscape(next), http.StatusFound)
}

// safeClaimNext keeps the post-claim redirect on the vhost itself. Browsers
// drop tabs and newlines from a Location, so "/\t/evil" would become "//evil",
// and http.Redirect cleans "/./\evil" into "/\evil", which they read the same.
func safeClaimNext(next string) string {
	path, _, _ := strings.Cut(next, "?")
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "\\") || strings.ContainsAny(next, "\t\n\r") {
		return "/"
	}
	return next
}
