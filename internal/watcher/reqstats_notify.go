package watcher

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/push"
	"github.com/geodro/lerd/internal/reqstats"
)

// slowRouteClearAfter is how many consecutive snapshots a warned route must be
// missing from the (truncated) slow list before it re-arms, so a route briefly
// bumped off the top by slower siblings doesn't fire a duplicate notification.
const slowRouteClearAfter = 3

// slowRouteNotifier turns newly-flagged slow routes into push notifications,
// edge-triggered: a route notifies once on crossing into the slow band and
// re-arms only after it stays off the list long enough to count as recovered.
type slowRouteNotifier struct {
	// absences counts consecutive snapshots a warned route has been missing from
	// the slow list; 0 means seen this round. A key present here is "warned".
	absences map[string]int
}

func newSlowRouteNotifier() *slowRouteNotifier {
	return &slowRouteNotifier{absences: map[string]int{}}
}

// notifications returns a push notification for each route newly gone slow, and
// clears a warned route only after slowRouteClearAfter snapshots off the list.
// domainOf resolves a site name to the domain used in the dashboard deep link.
func (n *slowRouteNotifier) notifications(snaps []reqstats.SiteStats, domainOf func(string) string) []push.Notification {
	current := make(map[string]bool)
	var out []push.Notification
	for _, s := range snaps {
		for _, r := range s.Slow {
			key := s.Site + "\x00" + r.Route
			current[key] = true
			if _, warned := n.absences[key]; warned {
				n.absences[key] = 0 // still slow: reset the absence streak
				continue
			}
			n.absences[key] = 0
			out = append(out, notificationForSlowRoute(s.Site, domainOf(s.Site), r))
		}
	}
	// Age out routes missing this round; clear only after a sustained absence so a
	// route briefly displaced from the truncated slow list doesn't re-notify.
	for key := range n.absences {
		if current[key] {
			continue
		}
		n.absences[key]++
		if n.absences[key] >= slowRouteClearAfter {
			delete(n.absences, key)
		}
	}
	return out
}

func notificationForSlowRoute(site, domain string, r reqstats.RouteStat) push.Notification {
	// With no usable baseline (an all-zero-ms site median) the multiplier is 0,
	// so fall back to the absolute phrasing instead of "0x slower than usual".
	body := fmt.Sprintf("%s p95 is %gms", r.Route, r.P95Millis)
	if r.Multiplier > 0 {
		body = fmt.Sprintf("%s is %gx slower than usual (%gms)", r.Route, r.Multiplier, r.P95Millis)
	}
	url := "#sites"
	if domain != "" {
		url = "#sites/" + domain + "/dumps"
	}
	return push.Notification{
		Kind:    "slow_route",
		Title:   "Slow route on " + site,
		Body:    body,
		Tag:     "lerd-slowroute-" + site + "-" + r.Route,
		URL:     url,
		Data:    map[string]string{"site": site, "route": r.Route},
		Urgency: "normal",
		TTL:     120,
	}
}

// siteDomainResolver loads the site registry once and returns a key->domain
// lookup, returning "" when the key has no domain so the caller can fall back to
// the sites list rather than deep-link a route that resolves to nothing. A
// worktree key resolves to the worktree's own domain, so the notification
// deep-links to the branch that went slow rather than to its parent.
func siteDomainResolver() func(string) string {
	reg, err := config.LoadSites()
	if err != nil {
		return func(string) string { return "" }
	}
	m := make(map[string]string, len(reg.Sites))
	for _, s := range reg.Sites {
		if len(s.Domains) > 0 {
			m[s.Name] = s.Domains[0]
		}
	}
	return func(key string) string {
		site, branch := reqstats.SplitKey(key)
		if branch != "" {
			if d := wtIndex.domainFor(site, branch); d != "" {
				return d
			}
		}
		return m[site]
	}
}
