package ui

import (
	"net/http"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/reqstats"
)

// statsRoute serves the request-timing view for a site: its typical response
// time and the routes running well above their own baseline. The data is
// collected by the watcher (the only process bound to the nginx access feed) and
// read here from config.RequestStatsFile(). Returns true when it owns the request.
//
//	GET /api/sites/{domain}/stats[?branch=<sanitized>]
func statsRoute(w http.ResponseWriter, r *http.Request, domain string, rest []string) bool {
	if len(rest) != 1 || rest[0] != "stats" || r.Method != http.MethodGet {
		return false
	}
	site, err := config.FindSiteByDomain(domain)
	if err != nil {
		writeJSON(w, map[string]any{"error": "site not found: " + domain})
		return true
	}
	key := reqstats.Key(site.Name, r.URL.Query().Get("branch"))
	stats, ok := reqstats.LoadSite(config.RequestStatsFile(), key)
	if !ok {
		// No traffic recorded yet: return an empty but well-formed view so the UI
		// renders its "watching for requests" state rather than an error.
		stats = reqstats.SiteStats{Site: key, Slow: []reqstats.RouteStat{}}
	}
	writeJSON(w, stats)
	return true
}
