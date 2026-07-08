package ui

import (
	"net/http"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/reqstats"
)

// analyticsStore is the read handle onto the durable request store the watcher
// writes. Opened once, lazily, since lerd-ui and the watcher are separate
// processes sharing the SQLite file (WAL, so the reader never blocks the writer).
var (
	analyticsStore     *reqstats.Store
	analyticsStoreErr  error
	analyticsStoreOnce sync.Once
)

func getAnalyticsStore() (*reqstats.Store, error) {
	analyticsStoreOnce.Do(func() {
		analyticsStore, analyticsStoreErr = reqstats.OpenStore(config.RequestStatsDB())
	})
	return analyticsStore, analyticsStoreErr
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
}

// analyticsResponse is the request-timing analytics view for one site over a
// window: the aggregate plus the tail of recent requests.
type analyticsResponse struct {
	reqstats.Analytics
	Range  string          `json:"range"`
	Recent []recentRequest `json:"recent"`
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
//	GET /api/sites/{domain}/analytics[?range=15m|1h|24h|7d][&branch=<sanitized>]
func analyticsRoute(w http.ResponseWriter, r *http.Request, domain string, rest []string) bool {
	if len(rest) != 1 || rest[0] != "analytics" || r.Method != http.MethodGet {
		return false
	}
	site, err := config.FindSiteByDomain(domain)
	if err != nil {
		writeJSON(w, map[string]any{"error": "site not found: " + domain})
		return true
	}
	key := site.Name
	if branch := r.URL.Query().Get("branch"); branch != "" {
		key = wtKey(site.Name, branch)
	}
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
	recent, _ := store.Recent(key, 20)
	out := analyticsResponse{Analytics: a, Range: rangeLabel, Recent: make([]recentRequest, 0, len(recent))}
	for _, rec := range recent {
		out.Recent = append(out.Recent, recentRequest{
			AtMillis: rec.At.UnixMilli(),
			Method:   rec.Method,
			Route:    rec.Route,
			URI:      rec.URI,
			Status:   rec.Status,
			Millis:   rec.Millis,
			Cold:     rec.Cold,
		})
	}
	writeJSON(w, out)
	return true
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
		Range:  rangeLabel,
		Recent: []recentRequest{},
	}
}
