package sitedoctor

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/reqstats"
)

// slowRoutesInDetail caps how many routes the finding names, so a site with many
// slow routes still reads as one short line rather than a wall of text.
const slowRoutesInDetail = 3

// checkSlowRoutes reports routes running well above the site's typical response
// time, read from the watcher's request-timing snapshot. It resolves the site
// from the project path, so worktree paths (which aren't registered sites) simply
// yield no finding. Returns ok=false when there's no site, no snapshot, or no
// flagged route, so a healthy or quiet site adds nothing to the report. The
// remedy is investigation, not a command, so no Fix is set.
func checkSlowRoutes(path string) (Check, bool) {
	site, err := config.FindSiteByPath(path)
	if err != nil || site == nil {
		return Check{}, false
	}
	stats, ok := reqstats.LoadSite(config.RequestStatsFile(), site.Name)
	if !ok || len(stats.Slow) == 0 {
		return Check{}, false
	}

	parts := make([]string, 0, slowRoutesInDetail)
	for _, r := range stats.Slow {
		if len(parts) == slowRoutesInDetail {
			break
		}
		parts = append(parts, fmt.Sprintf("%s (%gx, %gms)", r.Route, r.Multiplier, r.P95Millis))
	}
	detail := fmt.Sprintf("%d route(s) run well above this site's typical %gms response: %s.",
		len(stats.Slow), stats.MedianMillis, strings.Join(parts, ", "))
	if len(stats.Slow) > slowRoutesInDetail {
		detail += fmt.Sprintf(" (+%d more)", len(stats.Slow)-slowRoutesInDetail)
	}
	detail += " Profile them from the Request timing panel in the site's Debug tab."

	return Check{
		Name:   "slow_routes",
		Status: StatusWarn,
		Detail: detail,
	}, true
}
