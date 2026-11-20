package goserver

import (
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

const dashboardHost = "lerd.localhost"

// dashboardExact and dashboardPrefixes are the paths lerd.localhost hands to
// lerd-ui, the same list as nginx's vhost; a test reads it off that render.
var (
	dashboardExact    = []string{"/", "/manifest.webmanifest", "/sw.js", "/offline.html"}
	dashboardPrefixes = []string{"/icons/", "/assets/", "/_spx/", "/_svc/", "/docs/"}
)

func dashboardAllows(p string) bool {
	if slices.Contains(dashboardExact, p) {
		return true
	}
	for _, prefix := range dashboardPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	for _, mount := range config.DashboardMounts() {
		if strings.HasPrefix(p, mount) {
			return true
		}
	}
	return false
}

// serveDashboard hands lerd.localhost's allowed paths to lerd-ui and drops
// every other request without an answer, as nginx's return 444 does, so the
// API is only ever reached through lerd-ui's own listener.
func (h *Handler) serveDashboard(w http.ResponseWriter, r *http.Request) {
	if h.WakeHold == nil || !dashboardAllows(r.URL.Path) {
		dropConnection(w)
		return
	}
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(h.WakeHold)
			pr.Out.Host = r.Host
			pr.Out.Header.Set("X-Forwarded-For", forwardedFor(r))
			pr.Out.Header.Set("X-Real-IP", remoteIP(r))
			pr.Out.Header.Set("X-Forwarded-Proto", scheme(r))
		},
		Transport:     h.wakeTransport(),
		FlushInterval: -1,
	}
	rp.ServeHTTP(w, r)
}

func dropConnection(w http.ResponseWriter) {
	skipAccessFeed(w)
	if hj, ok := w.(http.Hijacker); ok {
		if conn, _, err := hj.Hijack(); err == nil {
			conn.Close()
			return
		}
	}
	panic(http.ErrAbortHandler)
}

// serveUnknownHost is nginx's default server: the 404 page from lerd's error
// pages for a host no site answers to.
func serveUnknownHost(w http.ResponseWriter) {
	page, err := os.ReadFile(filepath.Join(config.ErrorPagesDir(), "404.html"))
	if err != nil {
		http.Error(w, "404 page not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	w.Write(page)
}
