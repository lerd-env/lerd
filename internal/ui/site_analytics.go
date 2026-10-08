package ui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/reqstats"
	"github.com/geodro/lerd/internal/spxreport"
)

// getAnalyticsStore is the read handle onto the durable request store the
// watcher writes.
func getAnalyticsStore() (*reqstats.Store, error) {
	return reqstats.OpenShared(config.RequestStatsDB())
}

// loadSiteUsage reads per-key request counts and last-request times over the
// store's retention window, once per sites snapshot. Nil when the store isn't
// reachable, which leaves every site reading as untrafficked rather than failing
// the snapshot.
func loadSiteUsage() map[string]reqstats.SiteUsage {
	store, err := getAnalyticsStore()
	if err != nil {
		return nil
	}
	until := time.Now()
	usage, err := store.UsageBySite(until.Add(-reqstats.Retention), until)
	if err != nil {
		return nil
	}
	return usage
}

// addUsage folds a worktree's traffic into its site's, so a project driven from a
// worktree ranks by the work done on it rather than reading as untrafficked.
func addUsage(a, b reqstats.SiteUsage) reqstats.SiteUsage {
	a.Count += b.Count
	if b.LastAt.After(a.LastAt) {
		a.LastAt = b.LastAt
	}
	return a
}

// unixMilliOrZero renders a time for the sites payload, mapping the zero time to
// 0 so a site with no traffic omits the field rather than sending a 1970 stamp.
func unixMilliOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// recentRequest is one row of the recent-requests list: enough to render it
// without leaking the site key or absolute timestamps the UI doesn't need.
type recentRequest struct {
	AtMillis int64   `json:"at_millis"`
	Method   string  `json:"method"`
	Route    string  `json:"route"`
	URI      string  `json:"uri"`
	Status   int     `json:"status"`
	Millis   float64 `json:"millis"`
	Cold     bool    `json:"cold"`
	// RID links the row to what debug capture recorded for the request.
	RID string `json:"rid,omitempty"`
	// ProfileKey names the request's SPX capture, so its flame graph opens.
	ProfileKey string `json:"profile_key,omitempty"`
}

// analyticsResponse is the request-timing analytics view for one site over a
// window: the aggregate plus the tail of recent requests.
type analyticsResponse struct {
	reqstats.Analytics
	Range  string          `json:"range"`
	Recent []recentRequest `json:"recent"`
	// RecentMore says older requests exist past the page.
	RecentMore bool `json:"recent_more,omitempty"`
	// Excluded are the routes the user has silenced for this site. They carry in
	// the same payload as the data they are missing from, so the dashboard never
	// renders a view whose exclusions it hasn't caught up with.
	Excluded []string `json:"excluded"`
}

// analyticsRange maps a range label to its window, defaulting to the last hour
// for an absent or unknown value so the endpoint always answers.
func analyticsRange(s string) (time.Duration, string) {
	switch s {
	case "15m":
		return 15 * time.Minute, "15m"
	case "24h":
		return 24 * time.Hour, "24h"
	case "7d":
		return 7 * 24 * time.Hour, "7d"
	default:
		return time.Hour, "1h"
	}
}

// analyticsRoute serves the request-timing analytics view for a site over a
// window, read from the durable store the watcher fills from the nginx access
// feed. Returns true when it owns the request.
//
//	GET /api/sites/{domain}/analytics[?range=15m|1h|24h|7d][&branch=<sanitized>][&recent=N]
func analyticsRoute(w http.ResponseWriter, r *http.Request, domain string, rest []string) bool {
	if len(rest) == 2 && rest[0] == "analytics" {
		return analyticsMutateRoute(w, r, domain, rest[1])
	}
	if len(rest) != 1 || rest[0] != "analytics" || r.Method != http.MethodGet {
		return false
	}
	site, err := config.FindSiteByDomain(domain)
	if err != nil {
		writeJSON(w, map[string]any{"error": "site not found: " + domain})
		return true
	}
	key := reqstats.Key(site.Name, r.URL.Query().Get("branch"))
	dur, rangeLabel := analyticsRange(r.URL.Query().Get("range"))

	store, err := getAnalyticsStore()
	if err != nil {
		writeJSON(w, emptyAnalytics(key, rangeLabel))
		return true
	}
	until := time.Now()
	a, err := store.SiteAnalytics(key, until.Add(-dur), until)
	if err != nil {
		writeJSON(w, emptyAnalytics(key, rangeLabel))
		return true
	}
	// One past the page says whether there is more without counting the rest.
	limit := recentLimit(r.URL.Query().Get("recent"))
	recent, _ := store.Recent(key, limit+1)
	more := pageHasMore(len(recent), limit)
	if len(recent) > limit {
		recent = recent[:limit]
	}
	excluded, _ := store.ExcludedRoutes(key)
	if excluded == nil {
		excluded = []string{}
	}
	var captured map[string]bool
	if srv := dumpsServer.Load(); srv != nil {
		captured = srv.RequestIDs()
	}
	profiles := spxreport.KeysByRID(config.SpxDataDir())
	linkSlowest(a.Routes, captured, profiles)
	writeJSON(w, analyticsResponse{Analytics: a, Range: rangeLabel, Recent: recentRows(recent, captured, profiles), RecentMore: more, Excluded: excluded})
	return true
}

// forgetRequests drops the captured debug events and SPX profiles of requests
// whose history was just removed.
func forgetRequests(rids []string) {
	if srv := dumpsServer.Load(); srv != nil {
		srv.ForgetRequests(rids)
	}
	spxreport.RemoveForRIDs(config.SpxDataDir(), rids)
}

// linkSlowest links each route's slowest request to what is left of it, as
// recentRows does for the recent list.
func linkSlowest(routes []reqstats.RouteStat, captured map[string]bool, profiles map[string]string) {
	for _, r := range routes {
		if r.Slowest == nil {
			continue
		}
		r.Slowest.ProfileKey = profiles[r.Slowest.RID]
		if !captured[r.Slowest.RID] {
			r.Slowest.RID = ""
		}
	}
}

// recentLimitCap bounds one page of recent requests, so a single read stays cheap.
const recentLimitCap = 500

// recentLimit is how many recent requests a page asks for: 20 unless it says,
// and never more than the cap.
func recentLimit(q string) int {
	n, err := strconv.Atoi(q)
	if err != nil || n <= 0 {
		return 20
	}
	return min(n, recentLimitCap)
}

// pageHasMore reports whether a page fetched one row past its limit can offer
// another, which it cannot once it is at the cap.
func pageHasMore(fetched, limit int) bool {
	return fetched > limit && limit < recentLimitCap
}

// recentRows renders the recent-requests list. A row keeps its request id only
// while captured events for it are buffered, so Inspect never opens on nothing,
// and names its SPX capture whenever the request was profiled.
func recentRows(recent []reqstats.Record, captured map[string]bool, profiles map[string]string) []recentRequest {
	out := make([]recentRequest, 0, len(recent))
	for _, rec := range recent {
		row := recentRequest{
			AtMillis: rec.At.UnixMilli(),
			Method:   rec.Method,
			Route:    rec.Route,
			URI:      rec.URI,
			Status:   rec.Status,
			Millis:   rec.Millis,
			Cold:     rec.Cold,
		}
		if rec.RID != "" {
			row.ProfileKey = profiles[rec.RID]
		}
		if captured[rec.RID] {
			row.RID = rec.RID
		}
		out = append(out, row)
	}
	return out
}

// emptyAnalytics is a well-formed but empty view, so the UI renders its "watching
// for requests" state rather than an error when the store is unavailable or the
// site has no recorded traffic in the window.
func emptyAnalytics(key, rangeLabel string) analyticsResponse {
	return analyticsResponse{
		Analytics: reqstats.Analytics{
			Site:         key,
			Distribution: []reqstats.LatencyBucket{},
			Throughput:   []reqstats.ThroughputPoint{},
			Routes:       []reqstats.RouteStat{},
		},
		Range:    rangeLabel,
		Recent:   []recentRequest{},
		Excluded: []string{},
	}
}

// removeRequest is the body of a removal: which route to forget, optionally
// narrowed to the single request at AtMillis on URI (how the recent list
// identifies a row), and whether to stop recording the route from now on.
type removeRequest struct {
	Branch   string `json:"branch"`
	Route    string `json:"route"`
	AtMillis int64  `json:"at_millis"`
	URI      string `json:"uri"`
	Exclude  bool   `json:"exclude"`
}

// analyticsMutateRoute serves the write side of the request-timing view: dropping
// recorded history and managing the routes lerd stops watching. Returns true when
// it owns the request.
//
//	POST   /api/sites/{domain}/analytics/remove    {route, at_millis?, uri?, exclude, branch}
//	DELETE /api/sites/{domain}/analytics/excludes?route=<route>[&branch=<sanitized>]
func analyticsMutateRoute(w http.ResponseWriter, r *http.Request, domain, action string) bool {
	switch {
	case action == "remove" && r.Method == http.MethodPost:
	case action == "excludes" && r.Method == http.MethodDelete:
	default:
		return false
	}
	site, err := config.FindSiteByDomain(domain)
	if err != nil {
		http.Error(w, "site not found: "+domain, http.StatusNotFound)
		return true
	}
	store, err := getAnalyticsStore()
	if err != nil {
		http.Error(w, "request history is unavailable", http.StatusServiceUnavailable)
		return true
	}

	if action == "excludes" {
		route := r.URL.Query().Get("route")
		if route == "" {
			http.Error(w, "route is required", http.StatusBadRequest)
			return true
		}
		key := reqstats.Key(site.Name, r.URL.Query().Get("branch"))
		if err := store.UnexcludeRoute(key, route); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		writeJSON(w, map[string]any{"ok": true})
		return true
	}

	var req removeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return true
	}
	if req.Route == "" {
		http.Error(w, "route is required", http.StatusBadRequest)
		return true
	}
	key := reqstats.Key(site.Name, req.Branch)

	// A recent row names one request, so only that row goes; the route lists name
	// the whole route, so its history goes. Excluding is independent of either, and
	// applies from here on rather than reaching back over what is left.
	// The ids are read before the rows go, so what debug capture and SPX hold for
	// those requests can go with them.
	var removed int64
	var rids []string
	if req.AtMillis > 0 {
		rids, _ = store.RequestRIDs(key, req.AtMillis, req.URI)
		removed, err = store.DeleteRequest(key, req.AtMillis, req.URI)
	} else {
		rids, _ = store.RouteRIDs(key, req.Route)
		removed, err = store.DeleteRoute(key, req.Route)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	forgetRequests(rids)
	if req.Exclude {
		if err := store.ExcludeRoute(key, req.Route); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
	}
	writeJSON(w, map[string]any{"ok": true, "removed": removed})
	return true
}
