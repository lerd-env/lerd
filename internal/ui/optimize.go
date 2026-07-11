package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
	"github.com/geodro/lerd/internal/reqstats"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/geodro/lerd/internal/spxreport"
)

// spxTopN and spxMinPct bound the CPU hotspots attached to a slow route: the top
// few functions by exclusive wall time, and only those above a small share of the
// request, so the profile stays a few lines rather than a full flat profile.
const (
	spxTopN   = 8
	spxMinPct = 1.0
)

// RouteOptimization pairs one flagged slow route with the captured N+1 and
// slow-query evidence recorded against that same route, so a caller gets the
// symptom and its likely cause in one payload instead of correlating the timing
// snapshot and the query ring by hand.
type RouteOptimization struct {
	reqstats.RouteStat
	Evidence []RequestAnalysis  `json:"evidence,omitempty"`
	Profile  *spxreport.Profile `json:"profile,omitempty"`
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
	key := reqstats.Key(site, branch)
	stats, ok := reqstats.LoadSite(config.RequestStatsFile(), key)
	if !ok {
		stats = reqstats.SiteStats{Site: key}
	}
	var events []dumps.Event
	if srv := dumpsServer.Load(); srv != nil {
		events = srv.Filter(dumps.FilterOpts{Site: site, Branch: branch, Kind: dumps.KindQuery})
	}
	report := joinOptimize(stats, events, minRepeat, slowMS)
	attachProfiles(&report, site, branch)
	return report
}

// attachProfiles hangs the freshest SPX capture's top hotspots onto each slow
// route that has one, so a CPU-bound route shows where its time went next to its
// queries. Captures only exist when the profiler was on when the route was hit,
// so a route without one is left as-is. A worktree's captures carry its own
// subdomain as the host, so its report matches on that rather than the parent's.
func attachProfiles(report *OptimizeReport, site, branch string) {
	if len(report.Routes) == 0 {
		return
	}
	s, err := config.FindSite(site)
	if err != nil || len(s.Domains) == 0 {
		return
	}
	hosts := s.Domains
	if branch != "" {
		wtDomain, err := siteops.WorktreeDomain(s, branch)
		if err != nil {
			return
		}
		hosts = []string{wtDomain}
	}
	routes := make([]string, len(report.Routes))
	for i, r := range report.Routes {
		routes[i] = r.Route
	}
	profiles := spxreport.ProfilesForRoutes(config.SpxDataDir(), hosts, routes, spxTopN, spxMinPct)
	for i := range report.Routes {
		if p, ok := profiles[report.Routes[i].Route]; ok {
			report.Routes[i].Profile = &p
		}
	}
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
	key := reqstats.Key(resolveSiteName(q.Get("site")), q.Get("branch"))
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
