// Package browserlogs reports JavaScript errors from the pages lerd serves
// into the dashboard. A site that opts in gets a script injected into its HTML
// through nginx while debug capture is on; the script posts what it catches to
// a same-origin endpoint nginx proxies to lerd-ui, which records it.
package browserlogs

import (
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dashboard"
	"github.com/geodro/lerd/internal/dumps"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/siteops"
)

//go:embed browser.js
var scriptTemplate string

// The hooks SetSite, SaveSite and RefreshVhosts go through, swapped out in tests.
var (
	regenerateSiteVhostFn = regenerateSiteVhost
	nginxReloadFn         = nginx.Reload
)

// Capturable reports whether a site's vhost can carry the capture block:
// PHP-FPM, host-proxy and custom-container sites. FrankenPHP is left out for
// now, and a paused site or a sleeping host-proxy one keeps its holding page.
func Capturable(s config.Site) bool {
	if s.Ignored || s.Paused || s.IsFrankenPHP() {
		return false
	}
	return !(s.IsHostProxy() && slices.Contains(s.IdleSuspendedWorkers, config.HostProxyWorkerName))
}

// regenerateSiteVhost rewrites one site's vhost, and an FPM site's worktree
// vhosts too, which share its template.
func regenerateSiteVhost(s config.Site) error {
	if siteops.ServedByFPM(s) {
		return siteops.RegenerateFPMSiteVhosts(s)
	}
	return siteops.RegenerateSiteVhost(&s, s.PrimaryDomain())
}

// RefreshVhosts rewrites the vhosts of the sites that opted in, after the
// debug switch flipped, so their pages gain or lose the script. With no site
// opted in it does nothing, so the debug switch never reloads nginx for it.
func RefreshVhosts() error {
	reg, err := config.LoadSites()
	if err != nil {
		return err
	}
	rewritten := 0
	for _, s := range reg.Sites {
		if !Capturable(s) || !config.BrowserLogsFor(s).Enabled {
			continue
		}
		if err := regenerateSiteVhostFn(s); err != nil {
			return err
		}
		rewritten++
	}
	if rewritten == 0 {
		return nil
	}
	return nginxReloadFn()
}

// Result reports the outcome of a SetSite call.
type Result struct {
	Enabled  bool `json:"enabled"`
	NoChange bool `json:"no_change"`
}

// SetSite turns browser logs on or off for one site. Its pages carry the
// script only while debug capture is on as well.
func SetSite(site config.Site, on bool) (Result, error) {
	s := config.BrowserLogsFor(site)
	if s.Enabled == on {
		return Result{Enabled: on, NoChange: true}, nil
	}
	s.Enabled = on
	if err := SaveSite(site, s); err != nil {
		return Result{}, err
	}
	return Result{Enabled: on}, nil
}

// SaveSite stores a site's settings. Only turning the site on or off touches
// nginx, since the vhost carries that; everything else is read when the
// script is served.
func SaveSite(site config.Site, s config.BrowserLogsSettings) error {
	before := config.BrowserLogsFor(site)
	if err := config.SaveBrowserLogs(site, s); err != nil {
		return err
	}
	cfg, err := config.LoadGlobal()
	if err != nil || !cfg.IsDumpsEnabled() || !Capturable(site) {
		return err
	}
	updated, err := config.FindSite(site.Name)
	if err != nil {
		return err
	}
	after := config.BrowserLogsFor(*updated)
	if after.Enabled == before.Enabled {
		return nil
	}
	if err := regenerateSiteVhostFn(*updated); err != nil {
		return err
	}
	return nginxReloadFn()
}

// Script returns the capture script for a site's settings. A site with
// capture turned off gets a script that only says so, for a page cached from
// before it was turned off.
func Script(s config.BrowserLogsSettings, lensURL string) string {
	if !s.Enabled {
		return "console.info('lerd browser logs are off for this site');\n"
	}
	cfg, _ := json.Marshal(map[string]any{"console": s.Console, "network": s.Network, "navigation": s.Navigation, "resources": s.Resources, "events": s.Events, "endpoint": config.BrowserLogsPath, "lens": lensURL})
	js := strings.Replace(scriptTemplate, "__LERD_CONFIG__", string(cfg), 1)
	return js + ignoreListMap(strings.Count(js, "\n"))
}

// LensURL is the dashboard's Browser lens for a site, which the script names
// in the line it logs on load.
func LensURL(site config.Site) string {
	return dashboard.VhostURL + "/#sites/" + site.PrimaryDomain() + "/dumps/browser"
}

// ignoreListMap is an inline source map that marks the script as library code,
// so DevTools attributes a wrapped console.error to the app's own line rather
// than to lerd's wrapper. Each generated line maps onto itself.
func ignoreListMap(lines int) string {
	m, _ := json.Marshal(map[string]any{
		"version":             3,
		"sources":             []string{"lerd-browser-logs.js"},
		"names":               []string{},
		"mappings":            "AAAA" + strings.Repeat(";AACA", max(lines-1, 0)),
		"ignoreList":          []int{0},
		"x_google_ignoreList": []int{0},
	})
	return "//# sourceMappingURL=data:application/json;base64," + base64.StdEncoding.EncodeToString(m) + "\n"
}

// Report is one entry the script posts.
type Report struct {
	Type    string `json:"type"`
	Level   string `json:"level,omitempty"`
	Message string `json:"message"`
	Stack   string `json:"stack,omitempty"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Col     int    `json:"col,omitempty"`
	Method  string `json:"method,omitempty"`
	Request string `json:"request,omitempty"`
	Status  int    `json:"status,omitempty"`
	Nav     string `json:"nav,omitempty"`
	Tag     string `json:"tag,omitempty"`
	Name    string `json:"name,omitempty"`
	Label   string `json:"label,omitempty"`
	Cross   bool   `json:"cross,omitempty"`
	// RID is the PHP request a failed fetch or XHR reached, read off its
	// X-Lerd-Rid response header.
	RID  string `json:"rid,omitempty"`
	URL  string `json:"url,omitempty"`
	Page string `json:"page,omitempty"`
	UA   string `json:"ua,omitempty"`
	At   string `json:"at,omitempty"`
}

var reportTypes = map[string]bool{"error": true, "rejection": true, "console": true, "network": true, "navigation": true, "resource": true, "event": true}

// MaxReports caps how many entries one post may carry; the script itself
// stops after 50 per page.
const MaxReports = 50

// Events turns a posted batch into browser events for the site nginx named.
// Entries of a type lerd does not know are dropped.
func Events(body []byte, site, branch, host string) ([]dumps.Event, error) {
	var reports []Report
	if err := json.Unmarshal(body, &reports); err != nil {
		return nil, fmt.Errorf("decoding reports: %w", err)
	}
	if len(reports) > MaxReports {
		reports = reports[:MaxReports]
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	out := make([]dumps.Event, 0, len(reports))
	for _, r := range reports {
		if !reportTypes[r.Type] || r.Message == "" {
			continue
		}
		data, _ := json.Marshal(r)
		out = append(out, dumps.Event{
			V:     dumps.ProtocolVersion,
			ID:    newID(),
			TS:    now,
			Kind:  dumps.KindBrowser,
			Ctx:   dumps.Context{Type: "browser", Site: site, Branch: branch, Domain: host, Request: r.URL, RID: r.Page},
			Src:   dumps.Source{File: r.File, Line: r.Line},
			Label: r.Type,
			Data:  data,
		})
	}
	return out, nil
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// PresetStatus is a store preset as it stands for one site: whether the
// project uses its library, and whether its events are reported.
type PresetStatus struct {
	config.BrowserPreset
	Detected bool `json:"detected"`
	Active   bool `json:"active"`
}

// Presets lists the store presets for a site, the detected ones first and
// each group by label.
func Presets(site config.Site) []PresetStatus {
	settings := config.BrowserLogsFor(site)
	var out []PresetStatus
	for _, p := range config.BrowserPresets(site.Path) {
		detected := p.Detected(site.Path)
		out = append(out, PresetStatus{BrowserPreset: p, Detected: detected, Active: p.Active(settings, detected)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Detected != out[j].Detected {
			return out[i].Detected
		}
		return strings.ToLower(out[i].Label) < strings.ToLower(out[j].Label)
	})
	return out
}

// SetPreset switches a store preset on or off for a site.
func SetPreset(site config.Site, name string, on bool) error {
	if _, ok := config.FindBrowserPreset(site.Path, name); !ok {
		return fmt.Errorf("unknown browser logs preset %q", name)
	}
	s := config.BrowserLogsFor(site)
	s.Presets = maps.Clone(s.Presets)
	if s.Presets == nil {
		s.Presets = map[string]bool{}
	}
	s.Presets[name] = on
	return SaveSite(site, s)
}

// PageSettings are the settings the script is served with: the site's own,
// with the events of its active presets added.
func PageSettings(site config.Site) config.BrowserLogsSettings {
	s := config.BrowserLogsFor(site)
	presets := config.BrowserPresets(site.Path)
	s.Events = config.PageEvents(s, presets, func(p config.BrowserPreset) bool { return p.Active(s, p.Detected(site.Path)) })
	return s
}

// EventTypes are the types a browser event is filtered and counted by, with
// console messages split by level the way the dashboard filters them.
var EventTypes = []string{"error", "rejection", "console.error", "console.warn", "network", "resource", "event", "navigation"}

// EventType returns a browser event's type in EventTypes terms.
func EventType(r Report) string {
	if r.Type == "console" {
		return "console." + r.Level
	}
	return r.Type
}

// Entry is one browser event as an assistant reads it: only the fields that
// say what happened and where.
type Entry struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	At      string `json:"at,omitempty"`
	Stack   string `json:"stack,omitempty"`
	Method  string `json:"method,omitempty"`
	Request string `json:"request,omitempty"`
	Status  int    `json:"status,omitempty"`
	Cross   bool   `json:"cross_origin,omitempty"`
	RID     string `json:"rid,omitempty"`
	Time    string `json:"time"`
}

// PageView is one load of a page, or one SPA navigation, with what happened
// on it in order.
type PageView struct {
	URL    string  `json:"url"`
	Branch string  `json:"branch,omitempty"`
	Start  string  `json:"started"`
	Events []Entry `json:"events"`
}

// Summary is the browser events of a site grouped per page view, newest view
// first, with a count per type of everything that matched.
type Summary struct {
	Counts    map[string]int `json:"counts"`
	PageViews []PageView     `json:"page_views"`
}

// Summarize groups browser events per page view and keeps only the types
// asked for; none means all. Events arrive oldest first, as the ring holds them.
func Summarize(events []dumps.Event, types []string) Summary {
	out := Summary{Counts: map[string]int{}, PageViews: []PageView{}}
	index := map[string]int{}
	for _, e := range events {
		if e.Kind != dumps.KindBrowser {
			continue
		}
		var r Report
		if json.Unmarshal(e.Data, &r) != nil {
			continue
		}
		typ := EventType(r)
		if len(types) > 0 && !slices.Contains(types, typ) {
			continue
		}
		out.Counts[typ]++
		key := e.Ctx.RID
		if key == "" {
			key = e.ID
		}
		i, ok := index[key]
		if !ok {
			i = len(out.PageViews)
			index[key] = i
			out.PageViews = append(out.PageViews, PageView{URL: e.Ctx.Request, Branch: e.Ctx.Branch, Start: e.TS})
		}
		entry := Entry{Type: typ, Message: r.Message, Stack: r.Stack, Method: r.Method, Request: r.Request, Status: r.Status, Cross: r.Cross, RID: r.RID, Time: e.TS}
		if r.File != "" {
			entry.At = fmt.Sprintf("%s:%d:%d", r.File, r.Line, r.Col)
		}
		if typ == "event" && r.Label != "" {
			entry.Message = r.Name + ": " + entry.Message
		}
		out.PageViews[i].Events = append(out.PageViews[i].Events, entry)
	}
	slices.Reverse(out.PageViews)
	return out
}
