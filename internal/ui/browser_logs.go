package ui

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/geodro/lerd/internal/browserlogs"
	"github.com/geodro/lerd/internal/config"
)

// Paths the site vhosts proxy to lerd-ui while browser logs are on.
const (
	browserScriptPath = config.BrowserLogsPath + ".js"
	browserReportPath = config.BrowserLogsPath
)

// withBrowserLogs serves the capture script and endpoint ahead of the
// remote-control gate: they are reached from a site's own pages, a phone on
// the LAN included, and only through nginx, which names the site.
func withBrowserLogs(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != browserScriptPath && r.URL.Path != browserReportPath {
			next.ServeHTTP(w, r)
			return
		}
		if !fromNginx(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		site, err := config.FindSite(r.Header.Get("X-Lerd-Site"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		settings := config.BrowserLogsFor(*site)
		if r.URL.Path == browserScriptPath {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = io.WriteString(w, browserlogs.Script(browserlogs.PageSettings(*site), browserlogs.LensURL(*site)))
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !fromSitePage(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		srv := dumpsServer.Load()
		cfg, _ := config.LoadGlobal()
		if srv == nil || cfg == nil || !cfg.IsDumpsEnabled() || !settings.Enabled {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		events, err := browserlogs.Events(body, site.Name, r.Header.Get("X-Lerd-Branch"), r.Header.Get("X-Lerd-Host"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, e := range events {
			srv.Push(e)
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// handleBrowserLogsSite reads (GET) or replaces (POST) one site's
// settings at /api/browser-logs/sites/{site}.
func handleBrowserLogsSite(w http.ResponseWriter, r *http.Request) {
	name := resolveSiteName(strings.TrimPrefix(r.URL.Path, "/api/browser-logs/sites/"))
	site, err := config.FindSite(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		if !hasHostActionAuthority(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var s config.BrowserLogsSettings
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if s.Console == nil {
			s.Console = []string{}
		}
		if s.Network == nil {
			s.Network = []string{}
		}
		if err := browserlogs.SaveSite(*site, s); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if site, err = config.FindSite(name); err != nil {
			http.NotFound(w, r)
			return
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, config.BrowserLogsFor(*site))
}

// handleBrowserLogsPresets lists the store presets for ?site= (GET) or adds
// switches one on or off (POST {site, name, on}).
func handleBrowserLogsPresets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Site string `json:"site"`
		Name string `json:"name"`
		On   bool   `json:"on"`
	}
	switch r.Method {
	case http.MethodGet:
		req.Site = r.URL.Query().Get("site")
	case http.MethodPost:
		if !hasHostActionAuthority(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	site, err := config.FindSite(resolveSiteName(req.Site))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		if err := browserlogs.SetPreset(*site, req.Name, req.On); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if site, err = config.FindSite(site.Name); err != nil {
			http.NotFound(w, r)
			return
		}
	}
	presets := browserlogs.Presets(*site)
	if presets == nil {
		presets = []browserlogs.PresetStatus{}
	}
	writeJSON(w, presets)
}

// fromSitePage reports whether a report was posted by the site's own page.
// nginx names the site, but any page open in the browser can fire a simple
// POST at its endpoint, so the browser's provenance headers, which a page
// cannot set, must say the post came from the site itself. A client without
// them is not a browser page and could forge either header anyway.
func fromSitePage(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin"
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Hostname() == hostOnly(r.Header.Get("X-Lerd-Host"))
}

// hostOnly drops a port from a host header value.
func hostOnly(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}
