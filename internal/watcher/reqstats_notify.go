package watcher

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/push"
	"github.com/geodro/lerd/internal/reqstats"
)

// slowRouteNotifier turns newly-flagged slow routes into push notifications. It
// is edge-triggered: a route notifies once when it crosses into the slow band,
// and its warned state is cleared once it drops back within the site's typical
// response time, so a later slowdown of the same route notifies again. This
// keeps the kind low-volume without silencing a route forever.
type slowRouteNotifier struct {
	warned map[string]bool
}

func newSlowRouteNotifier() *slowRouteNotifier {
	return &slowRouteNotifier{warned: map[string]bool{}}
}

// notifications returns a push notification for each route that has become slow
// since a prior call, and clears any previously-warned route that is no longer
// flagged (it recovered). domainOf resolves a site name to the domain used in
// the dashboard deep link.
func (n *slowRouteNotifier) notifications(snaps []reqstats.SiteStats, domainOf func(string) string) []push.Notification {
	current := make(map[string]bool)
	var out []push.Notification
	for _, s := range snaps {
		for _, r := range s.Slow {
			key := s.Site + "\x00" + r.Route
			current[key] = true
			if n.warned[key] {
				continue
			}
			n.warned[key] = true
			out = append(out, notificationForSlowRoute(s.Site, domainOf(s.Site), r))
		}
	}
	// Drop routes that fell back within the typical band so the next slowdown
	// fires a fresh notification.
	for key := range n.warned {
		if !current[key] {
			delete(n.warned, key)
		}
	}
	return out
}

func notificationForSlowRoute(site, domain string, r reqstats.RouteStat) push.Notification {
	return push.Notification{
		Kind:    "slow_route",
		Title:   "Slow route on " + site,
		Body:    fmt.Sprintf("%s is %gx slower than usual (%gms)", r.Route, r.Multiplier, r.P95Millis),
		Tag:     "lerd-slowroute-" + site + "-" + r.Route,
		URL:     "#sites/" + domain + "/dumps",
		Data:    map[string]string{"site": site, "route": r.Route},
		Urgency: "normal",
		TTL:     120,
	}
}

// siteDomainResolver loads the site registry once and returns a name->domain
// lookup, falling back to the site name when it has no domain.
func siteDomainResolver() func(string) string {
	reg, err := config.LoadSites()
	if err != nil {
		return func(s string) string { return s }
	}
	m := make(map[string]string, len(reg.Sites))
	for _, s := range reg.Sites {
		if len(s.Domains) > 0 {
			m[s.Name] = s.Domains[0]
		}
	}
	return func(site string) string {
		if d, ok := m[site]; ok {
			return d
		}
		return site
	}
}
