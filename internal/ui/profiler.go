package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/profiler"
)

// handleProfilerToggle turns the SPX profiler on or off globally. Loopback-
// only: it rewrites every PHP-FPM site's nginx vhost.
func handleProfilerToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	res, err := profiler.SetProfiling(req.Enable)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

// handleProfilerStatus reports whether the SPX profiler is globally armed.
func handleProfilerStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(buildProfilerStatusJSON())
}

// buildProfilerStatusJSON renders the same payload as handleProfilerStatus so
// the websocket broker can ship it as a profiler_status frame.
func buildProfilerStatusJSON() []byte {
	cfg, _ := config.LoadGlobal()
	b, _ := json.Marshal(map[string]bool{"enabled": cfg != nil && cfg.IsProfilerEnabled()})
	return b
}

// handleProfilerClear deletes every captured SPX report. Loopback-only: it
// removes files from the shared profiler data directory.
func handleProfilerClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	removed, err := profiler.ClearData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]int{"removed": removed})
}

// spxStripPrefix removes the /_spx mount prefix so the proxied request path
// matches what the profiler.localhost vhost expects.
func spxStripPrefix(p string) string {
	p = strings.TrimPrefix(p, "/_spx")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// newSpxProxy builds the reverse proxy that serves the SPX UI same-origin with
// the dashboard. Targeting nginx's profiler.localhost vhost puts the UI inside
// the dashboard origin, which lets the Profiler overlay drive the iframe (back,
// reload) directly instead of guessing across an origin boundary.
func newSpxProxy() *httputil.ReverseProxy {
	httpPort := 80
	if cfg, err := config.LoadGlobal(); err == nil && cfg.Nginx.HTTPPort > 0 {
		httpPort = cfg.Nginx.HTTPPort
	}
	target := &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", httpPort)}
	proxy := httputil.NewSingleHostReverseProxy(target)
	orig := proxy.Director
	proxy.Director = func(req *http.Request) {
		orig(req)
		req.URL.Path = spxStripPrefix(req.URL.Path)
		req.Host = "profiler.localhost"
		// The $spx_key nginx map blanks the SPX key when X-Forwarded-Host is
		// set, which would lock the UI out — strip it.
		req.Header.Del("X-Forwarded-Host")
	}
	// SPX serves no cache headers, so browsers heuristically cache the report
	// list and keep showing a stale (often empty) table. Forbid caching.
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("Cache-Control", "no-store")
		resp.Header.Del("ETag")
		resp.Header.Del("Last-Modified")
		return nil
	}
	return proxy
}

var spxProxy = sync.OnceValue(newSpxProxy)

// handleSpxProxy serves the SPX profiler UI same-origin under /_spx/.
func handleSpxProxy(w http.ResponseWriter, r *http.Request) {
	spxProxy().ServeHTTP(w, r)
}
