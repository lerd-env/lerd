package ui

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/activityping"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
)

// The seams the wake hold goes through, vars so tests stand in for the watcher
// and the vhost on disk.
var (
	wakeHoldPing   = activityping.Site
	wakeHoldAsleep = siteVhostWaking
	wakeHoldPoll   = 50 * time.Millisecond
	wakeHoldMax    = 60 * time.Second
	// wakeHoldSettle covers the nginx master applying a reload it was just
	// signalled to do; the signal returns before the new workers take over.
	wakeHoldSettle = 50 * time.Millisecond
)

// withWakeHold serves the wake hold ahead of the remote-control gate: it only
// waits and redirects a request back to the path it came from, and a sleeping
// site reached over the LAN has to wake the same as one reached locally.
func withWakeHold(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == nginx.WakeHoldPath {
			handleWakeHold(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// wakeHoldFailed is the status the hold answers when it cannot serve the
// request itself. nginx turns only this code (and its own 502/504) into the
// static waking page, so an app's own 404 or 500 reaches the client untouched.
const wakeHoldFailed = 599

// wakeHoldReplayed marks a request the hold sent on, so one that lands back in
// the hold (nginx not serving the restored vhost yet) is not replayed again.
const wakeHoldReplayed = "X-Lerd-Replayed"

// handleWakeHold is where a sleeping site's vhost sends each request. It wakes
// the site (the access log only records a request once it finishes, so the
// watcher would not hear of a held one otherwise), waits until nginx serves the
// real vhost again, then sends the request on to it and returns the app's own
// response. Any client works that way, a webhook or an API call as much as a
// browser. Only a websocket upgrade, which cannot be replayed, is redirected.
func handleWakeHold(w http.ResponseWriter, r *http.Request) {
	host := r.Header.Get("X-Lerd-Wake-Host")
	uri := r.Header.Get("X-Lerd-Wake-Uri")
	// A relative path only, so the request can never leave the site.
	if host == "" || !strings.HasPrefix(uri, "/") || strings.HasPrefix(uri, "//") || r.Header.Get(wakeHoldReplayed) != "" {
		w.WriteHeader(wakeHoldFailed)
		return
	}
	site, err := config.FindSiteByDomain(host)
	if err != nil || site == nil {
		w.WriteHeader(wakeHoldFailed)
		return
	}
	wakeHoldPing(site.Name)
	deadline := time.Now().Add(wakeHoldMax)
	for wakeHoldAsleep(site.PrimaryDomain()) {
		if time.Now().After(deadline) {
			w.WriteHeader(wakeHoldFailed)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(wakeHoldPoll):
		}
	}
	time.Sleep(wakeHoldSettle)
	if r.Header.Get("Upgrade") != "" {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Location", uri)
		w.WriteHeader(http.StatusTemporaryRedirect)
		return
	}
	replayToSite(w, r, host, uri, r.Header.Get("X-Lerd-Wake-Scheme"))
}

// wakeHoldAddr is where nginx answers for scheme on this host; a var so tests
// point the replay at a stand-in.
var wakeHoldAddr = func(scheme string) string {
	httpPort, httpsPort := config.NginxPorts()
	if scheme == "https" {
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(httpsPort))
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(httpPort))
}

// replayToSite sends the held request to nginx as the client made it (method,
// path, headers, body) and streams the app's response back. TLS is spoken to
// lerd's own nginx on loopback under the site's name, so its certificate is
// not checked against a CA the lerd-ui process may not trust.
func replayToSite(w http.ResponseWriter, r *http.Request, host, uri, scheme string) {
	if scheme != "https" {
		scheme = "http"
	}
	addr := wakeHoldAddr(scheme)
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = scheme
			pr.Out.URL.Host = host
			pr.Out.Host = host
			if u, err := url.ParseRequestURI(uri); err == nil {
				pr.Out.URL.Path, pr.Out.URL.RawPath, pr.Out.URL.RawQuery = u.Path, u.RawPath, u.RawQuery
			}
			for k := range pr.Out.Header {
				if strings.HasPrefix(k, "X-Lerd-Wake-") {
					pr.Out.Header.Del(k)
				}
			}
			pr.Out.Header.Set(wakeHoldReplayed, "1")
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, addr)
			},
			TLSClientConfig: &tls.Config{ServerName: host, InsecureSkipVerify: true}, //nolint:gosec // loopback to lerd's own nginx
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			w.WriteHeader(wakeHoldFailed)
		},
	}
	proxy.ServeHTTP(w, r)
}

// siteVhostWaking reports whether requests to the site still land in the wake
// hold: its vhost still hands them here, or it has been restored but nginx has
// not reloaded since. Redirecting before that reload bounces the client straight
// back, once per poll, while every other site woken with it is restored.
func siteVhostWaking(domain string) bool {
	path := filepath.Join(config.NginxConfD(), domain+".conf")
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), nginx.WakeHoldPath) {
		return err == nil
	}
	return !nginx.ServesVhostWrittenAt(st.ModTime())
}
