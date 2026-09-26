package ui

import (
	"net/http"
	"os"
	"path/filepath"
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

// handleWakeHold is where a sleeping site's vhost sends each request. It wakes
// the site (the access log only records a request once it finishes, so the
// watcher would not hear of a held one otherwise), waits until the real vhost is
// back, then sends the client to the same URL with a 307, which keeps the
// method and body. Anything else answers with an error nginx turns into the
// static waking page.
func handleWakeHold(w http.ResponseWriter, r *http.Request) {
	host := r.Header.Get("X-Lerd-Wake-Host")
	uri := r.Header.Get("X-Lerd-Wake-Uri")
	// A relative path only, so the redirect can never leave the site.
	if host == "" || !strings.HasPrefix(uri, "/") || strings.HasPrefix(uri, "//") {
		http.NotFound(w, r)
		return
	}
	site, err := config.FindSiteByDomain(host)
	if err != nil || site == nil {
		http.NotFound(w, r)
		return
	}
	wakeHoldPing(site.Name)
	deadline := time.Now().Add(wakeHoldMax)
	for wakeHoldAsleep(site.PrimaryDomain()) {
		if time.Now().After(deadline) {
			http.Error(w, "still waking", http.StatusServiceUnavailable)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(wakeHoldPoll):
		}
	}
	time.Sleep(wakeHoldSettle)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", uri)
	w.WriteHeader(http.StatusTemporaryRedirect)
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
