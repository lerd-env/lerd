package ui

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/geodro/lerd/internal/browsercapture"
	"github.com/geodro/lerd/internal/config"
)

// Paths the site vhosts proxy to lerd-ui while browser capture is on.
const (
	browserScriptPath = "/_lerd/browser.js"
	browserReportPath = "/_lerd/browser"
)

// withBrowserCapture serves the capture script and endpoint ahead of the
// remote-control gate: they are reached from a site's own pages, a phone on
// the LAN included, and only through nginx, which names the site.
func withBrowserCapture(next http.Handler) http.Handler {
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
		settings := config.BrowserCaptureFor(*site)
		if r.URL.Path == browserScriptPath {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = io.WriteString(w, browsercapture.Script(settings))
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		srv := dumpsServer.Load()
		cfg, _ := config.LoadGlobal()
		if srv == nil || cfg == nil || !cfg.IsBrowserCaptureEnabled() || !settings.Enabled {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		events, err := browsercapture.Events(body, site.Name, r.Header.Get("X-Lerd-Branch"), r.Header.Get("X-Lerd-Host"))
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

// handleBrowserCaptureStatus reports whether browser capture is on.
func handleBrowserCaptureStatus(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.LoadGlobal()
	writeJSON(w, map[string]bool{"enabled": cfg != nil && cfg.IsBrowserCaptureEnabled()})
}

// handleBrowserCaptureToggle turns browser capture on or off globally. It
// rewrites every FPM site's vhost, so it takes host-action authority.
func handleBrowserCaptureToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !hasHostActionAuthority(r) {
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
	res, err := browsercapture.SetEnabled(req.Enable)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

// browserCaptureSiteResponse is a site's settings plus where they are kept,
// so the dashboard can say whether a change lands in .lerd.yaml.
type browserCaptureSiteResponse struct {
	config.BrowserCaptureSettings
	Source string `json:"source"`
}

// handleBrowserCaptureSite reads (GET) or replaces (POST) one site's
// settings at /api/browser-capture/sites/{site}.
func handleBrowserCaptureSite(w http.ResponseWriter, r *http.Request) {
	name := resolveSiteName(strings.TrimPrefix(r.URL.Path, "/api/browser-capture/sites/"))
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
		var s config.BrowserCaptureSettings
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
		if err := browsercapture.SaveSite(*site, s); err != nil {
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
	source := "registry"
	if config.BrowserCaptureInProjectFile(*site) {
		source = "lerd.yaml"
	}
	writeJSON(w, browserCaptureSiteResponse{BrowserCaptureSettings: config.BrowserCaptureFor(*site), Source: source})
}

// handleBrowserCapturePresets lists the store presets for ?site= (GET) or adds
// or removes one (POST {site, name, add}).
func handleBrowserCapturePresets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Site string `json:"site"`
		Name string `json:"name"`
		Add  bool   `json:"add"`
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
		if err := browsercapture.ApplyPreset(*site, req.Name, req.Add); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if site, err = config.FindSite(site.Name); err != nil {
			http.NotFound(w, r)
			return
		}
	}
	presets := browsercapture.Presets(*site)
	if presets == nil {
		presets = []browsercapture.PresetStatus{}
	}
	writeJSON(w, presets)
}
