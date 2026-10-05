package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/geodro/lerd/internal/browsercapture"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
)

// debugbarPrefix is where the site vhosts proxy the debug bar's script and
// data while a site shows it.
const debugbarPrefix = "/_lerd/bar/"

// debugbarBundle is the built bar, read once from the embedded dist.
var debugbarBundle = sync.OnceValues(func() ([]byte, error) {
	return fs.ReadFile(svelteFS(), "debugbar.js")
})

// withDebugbar serves the debug bar ahead of the remote-control gate, like
// browser capture: it is reached from a site's own pages, only through nginx,
// which names the site, and answers for that site alone.
func withDebugbar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, debugbarPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		if !fromNginx(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		site, err := config.FindSite(r.Header.Get("X-Lerd-Site"))
		if err != nil || !config.DebugbarFor(*site) {
			http.NotFound(w, r)
			return
		}
		switch p := strings.TrimPrefix(r.URL.Path, debugbarPrefix); {
		case p == "bar.js":
			serveDebugbarScript(w, r, *site)
		case strings.HasPrefix(p, "requests/"):
			serveDebugbarRequest(w, r, site.Name, strings.TrimPrefix(p, "requests/"))
		case p == "annotations" || strings.HasPrefix(p, "annotations/"):
			if !debugbarLocal(r) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			serveDebugbarAnnotations(w, r, site.Name, strings.TrimPrefix(strings.TrimPrefix(p, "annotations"), "/"))
		case p == "source" || p == "open-editor":
			if !debugbarLocal(r) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			if p == "source" {
				writeSource(w, r)
				return
			}
			serveDebugbarOpenEditor(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// siteRoots are the folders of the linked sites, so a path in another site, an
// API the page called, reads from its project's root too.
func siteRoots() []string {
	roots := []string{}
	if reg, err := config.LoadSites(); err == nil {
		for _, s := range reg.Sites {
			if s.Path != "" {
				roots = append(roots, s.Path)
			}
		}
	}
	return roots
}

// debugbarLocal reports whether a bar request can only have come from this
// machine: with the LAN not exposed nginx listens on loopback alone, a tunnel
// that shares the site marks what it forwards, and only the page's own script
// sends a same-origin fetch. Reading a site's code and opening the editor need
// all three, since nginx cannot tell one local client from another.
func debugbarLocal(r *http.Request) bool {
	if cfg, err := config.LoadGlobal(); err != nil || cfg.LAN.Exposed {
		return false
	}
	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "Forwarded", "Cf-Connecting-Ip", "X-Real-Ip"} {
		if r.Header.Get(h) != "" {
			return false
		}
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin"
	}
	// A page served over plain http gets no Sec-Fetch headers, so its own
	// origin, or the page it came from, has to be the site itself.
	host := r.Header.Get("X-Lerd-Host")
	if host == "" {
		host = r.Host
	}
	for _, h := range []string{"Origin", "Referer"} {
		if u, err := url.Parse(r.Header.Get(h)); err == nil && u.Host != "" {
			return u.Hostname() == strings.Split(host, ":")[0]
		}
	}
	return false
}

// serveDebugbarOpenEditor opens a file of a linked site in the editor, for a
// path link or a line number in the bar.
func serveDebugbarOpenEditor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Path string `json:"path"`
		Line int    `json:"line"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	path, ok := siteSourcePath(req.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	openEditorAt(w, path, req.Line)
}

// serveDebugbarScript hands out the bar with the global settings baked in,
// wrapped so nothing lands on the page's window.
func serveDebugbarScript(w http.ResponseWriter, r *http.Request, site config.Site) {
	bundle, err := debugbarBundle()
	if err != nil {
		http.Error(w, "debug bar build missing", http.StatusInternalServerError)
		return
	}
	cfg, _ := config.LoadGlobal()
	settings := config.Debugbar{}
	theme := ""
	if cfg != nil {
		settings, theme = cfg.Debugbar, cfg.UI.Theme
	}
	themes, _ := config.UIThemes()
	conf, _ := json.Marshal(struct {
		config.Debugbar
		Palette string           `json:"palette"`
		Base    string           `json:"base"`
		Site    string           `json:"site"`
		Path    string           `json:"path"`
		Roots   []string         `json:"roots"`
		Local   bool             `json:"local"`
		Themes  []config.UITheme `json:"themes"`
	}{settings.Resolve(), theme, config.BrowserCaptureFor(site).Route + "/bar/", site.Name, site.Path, siteRoots(), cfg != nil && !cfg.LAN.Exposed, withDesktopTheme(themes)})
	body := "(function(__lerdBarConfig){" + string(bundle) + "\n})(" + string(conf) + ");\n"
	sum := sha256.Sum256([]byte(body))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write([]byte(body))
}

// serveDebugbarRequest answers one request's detail, the same the dashboard
// reads, but only for a request the asking site served or one of its pages
// sent, an API on another site included.
func serveDebugbarRequest(w http.ResponseWriter, r *http.Request, site, rid string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	srv := dumpsServer.Load()
	var events []dumps.Event
	if srv != nil {
		events = srv.Lite()
	}
	d, ok := requestDetail(events, rid)
	if !ok || (d.Site != site && (d.Parent == nil || d.Parent.Site != site)) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Query().Get("summary") != "" {
		writeJSON(w, barSummary(d))
		return
	}
	writeJSON(w, withTraces(srv, d))
}

// barSummary trims a detail to what the bar's chips read, which it polls; the
// counts stand in for the events, and the full detail waits for the panel.
func barSummary(d RequestDetail) RequestDetail {
	events := map[string][]dumps.Event{}
	for _, kind := range []string{"request", "auth", "tab"} {
		if evs, ok := d.Events[kind]; ok {
			events[kind] = evs
		}
	}
	d.Events = events
	if d.Queries != nil {
		d.Queries = &RequestAnalysis{QueryCount: d.Queries.QueryCount, TotalTimeMS: d.Queries.TotalTimeMS}
	}
	return d
}

// handleDebugbarSite reads (GET) or sets (POST {enable}) whether one site
// shows the debug bar, at /api/debugbar/sites/{site}.
func handleDebugbarSite(w http.ResponseWriter, r *http.Request) {
	site, err := config.FindSite(resolveSiteName(strings.TrimPrefix(r.URL.Path, "/api/debugbar/sites/")))
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
		var req struct {
			Enable bool `json:"enable"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if _, err := browsercapture.SetDebugbar(*site, req.Enable); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if site, err = config.FindSite(site.Name); err != nil {
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
	writeJSON(w, map[string]any{"enabled": config.DebugbarFor(*site), "source": source})
}

// handleDebugbarSettings reads (GET) or replaces (POST) how the bar looks on
// every site that shows it.
func handleDebugbarSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		if !hasHostActionAuthority(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var d config.Debugbar
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := d.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg.Debugbar = d
		if err := config.SaveGlobal(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, cfg.Debugbar.Resolve())
}
