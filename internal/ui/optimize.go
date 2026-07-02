package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
	"github.com/geodro/lerd/internal/reqstats"
)

// RouteOptimization pairs one flagged slow route with the captured N+1 and
// slow-query evidence recorded against that same route, so a caller gets the
// symptom and its likely cause in one payload instead of correlating the timing
// snapshot and the query ring by hand.
type RouteOptimization struct {
	reqstats.RouteStat
	Evidence []RequestAnalysis `json:"evidence,omitempty"`
}

// OptimizeReport is the joined per-site view: the response-time baseline and
// every slow route with the query findings behind it.
type OptimizeReport struct {
	Site         string              `json:"site"`
	MedianMillis float64             `json:"median_millis"`
	Samples      int                 `json:"samples"`
	Routes       []RouteOptimization `json:"routes"`
}

// resolveSiteName maps a site identifier that may be a domain (astrolov.test) to
// the internal site name (astrolov) the dumps ring and reqstats key on, so a
// caller can pass either. Shares one resolver with the MCP dispatch boundary.
func resolveSiteName(s string) string {
	return config.ResolveSiteRef(s)
}

// joinOptimize buckets captured-query findings onto the slow routes by the shared
// reqstats route normalizer, so a route and its N+1s line up on the same key.
// Pure (no I/O) so the join is unit-testable.
func joinOptimize(stats reqstats.SiteStats, events []dumps.Event, minRepeat int, slowMS float64) OptimizeReport {
	report := OptimizeReport{
		Site: stats.Site, MedianMillis: stats.MedianMillis, Samples: stats.Samples,
		Routes: []RouteOptimization{},
	}
	byRoute := map[string][]RequestAnalysis{}
	for _, ra := range analyzeQueries(events, minRepeat, slowMS).Requests {
		if ra.Request == "" {
			continue // worker invocations have no route to attach to
		}
		method, path, _ := strings.Cut(ra.Request, " ")
		key := reqstats.NormalizeRoute(method, path)
		byRoute[key] = append(byRoute[key], ra)
	}
	for _, rs := range stats.Slow {
		report.Routes = append(report.Routes, RouteOptimization{RouteStat: rs, Evidence: byRoute[rs.Route]})
	}
	return report
}

// optimizeSite loads the watcher's slow-route snapshot and the captured-query
// ring for a site and joins them. branch selects a git worktree's stats.
func optimizeSite(site, branch string, minRepeat int, slowMS float64) OptimizeReport {
	key := site
	if branch != "" {
		key = wtKey(site, branch)
	}
	stats, ok := reqstats.LoadSite(config.RequestStatsFile(), key)
	if !ok {
		stats = reqstats.SiteStats{Site: key}
	}
	var events []dumps.Event
	if srv := dumpsServer.Load(); srv != nil {
		events = srv.Filter(dumps.FilterOpts{Site: site, Branch: branch, Kind: dumps.KindQuery})
	}
	return joinOptimize(stats, events, minRepeat, slowMS)
}

// handleRouteTiming serves the per-site request-timing snapshot (median + slow
// routes) keyed by site name, matching how analyze_queries is addressed.
//
//	GET /api/queries/route-timing?site=<name>[&branch=<sanitized>]
func handleRouteTiming(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	key := resolveSiteName(q.Get("site"))
	if branch := q.Get("branch"); branch != "" {
		key = wtKey(key, branch)
	}
	stats, ok := reqstats.LoadSite(config.RequestStatsFile(), key)
	if !ok {
		stats = reqstats.SiteStats{Site: key, Slow: []reqstats.RouteStat{}}
	}
	writeJSON(w, stats)
}

// handleOptimize serves the joined slow-route + query-evidence report.
//
//	GET /api/queries/optimize?site=<name>[&branch=][&min_repeat=][&slow_ms=]
func handleOptimize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	minRepeat, _ := strconv.Atoi(q.Get("min_repeat"))
	slowMS, _ := strconv.ParseFloat(q.Get("slow_ms"), 64)
	writeJSON(w, optimizeSite(resolveSiteName(q.Get("site")), q.Get("branch"), minRepeat, slowMS))
}
