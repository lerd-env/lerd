package ui

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/push"
	"github.com/geodro/lerd/internal/workerheal"
)

// siteDomainForRoute resolves the site an event or worker names to its primary
// domain, which the dashboard's hash router keys the Sites tab by. "" when no
// registered site matches, so the caller links to the sites list rather than
// to a site that does not exist.
func siteDomainForRoute(name string) string {
	if s := registeredSite(name); s != nil {
		return s.PrimaryDomain()
	}
	return ""
}

// registeredSite finds the site a name belongs to: the site's own name, a
// worktree worker's "<site>/<worktree folder>", or the project folder a CLI run
// without LERD_SITE falls back to, which differs from the name when the site
// was linked under another one.
func registeredSite(name string) *config.Site {
	parent, _, _ := strings.Cut(name, "/")
	if parent == "" {
		return nil
	}
	reg, err := config.LoadSites()
	if err != nil {
		return nil
	}
	for _, s := range reg.Sites {
		if s.Name == parent {
			return &s
		}
	}
	for _, s := range reg.Sites {
		if filepath.Base(s.Path) == parent {
			return &s
		}
	}
	return nil
}

// newEventSiteNamer returns the debug receiver's site resolver: an event that
// names a site's folder is put back on that site, so it shows in the site's
// Debug tab. The registry is reread at most once per ttl, since query events
// arrive by the hundred.
func newEventSiteNamer(ttl time.Duration) func(string) string {
	var mu sync.Mutex
	var loaded time.Time
	byFolder := map[string]string{}
	return func(name string) string {
		if name == "" || strings.Contains(name, "/") {
			return name
		}
		mu.Lock()
		defer mu.Unlock()
		if time.Since(loaded) > ttl {
			loaded = time.Now()
			byFolder = map[string]string{}
			if reg, err := config.LoadSites(); err == nil {
				names := map[string]bool{}
				for _, s := range reg.Sites {
					names[s.Name] = true
				}
				for _, s := range reg.Sites {
					if f := filepath.Base(s.Path); !names[f] {
						byFolder[f] = s.Name
					}
				}
			}
		}
		if site, ok := byFolder[name]; ok {
			return site
		}
		return name
	}
}

// newWorkerFailures returns workers in cur whose Unit names weren't in prev.
// Identity by unit only — a state change on a known-failed unit doesn't
// fire a fresh notification.
func newWorkerFailures(prev, cur []workerheal.UnhealthyWorker) []workerheal.UnhealthyWorker {
	if len(cur) == 0 {
		return nil
	}
	prevSet := make(map[string]struct{}, len(prev))
	for _, p := range prev {
		prevSet[p.Unit] = struct{}{}
	}
	var out []workerheal.UnhealthyWorker
	for _, c := range cur {
		if _, seen := prevSet[c.Unit]; !seen {
			out = append(out, c)
		}
	}
	return out
}

func notificationForWorkerFailure(w workerheal.UnhealthyWorker) push.Notification {
	site := w.Site
	if site == "" {
		site = w.Unit
	}
	worker := w.Worker
	if worker == "" {
		worker = w.Unit
	}
	state := workerheal.HumanState(w.State)
	return push.Notification{
		Kind:     "worker_failed",
		TitleKey: "notify_worker_failed_title",
		Title:    "Worker needs healing on " + site,
		BodyKey:  "notify_worker_failed_body",
		Body:     worker + " is " + state + ". Open lerd to heal.",
		Params:   map[string]string{"site": site, "worker": worker, "state": state},
		Tag:      "lerd-worker-" + w.Unit,
		URL:      "#sites/" + siteDomainForRoute(site),
		Data:     map[string]string{"unit": w.Unit, "site": site},
		Urgency:  "high",
		TTL:      300,
	}
}

// notificationForWorkerFailures collapses a batch of new failures into a
// single push payload. A one-element batch falls through to the per-unit
// shape so existing tag-based dedupe on a single worker still works; two
// or more failures get a grouped title/body and a stable group tag so a
// later supersedes-an-earlier grouped push doesn't pile up.
func notificationForWorkerFailures(ws []workerheal.UnhealthyWorker) push.Notification {
	if len(ws) == 1 {
		return notificationForWorkerFailure(ws[0])
	}
	siteSet := make(map[string]struct{}, len(ws))
	entries := make([]string, 0, len(ws))
	for _, w := range ws {
		site := w.Site
		if site == "" {
			site = w.Unit
		}
		worker := w.Worker
		if worker == "" {
			worker = w.Unit
		}
		siteSet[site] = struct{}{}
		entries = append(entries, worker+"@"+site)
	}
	sort.Strings(entries)
	sites := make([]string, 0, len(siteSet))
	for s := range siteSet {
		sites = append(sites, s)
	}
	sort.Strings(sites)
	count := strconv.Itoa(len(ws))
	workers := strings.Join(entries, ", ")
	return push.Notification{
		Kind:     "worker_failed",
		TitleKey: "notify_worker_failed_group_title",
		Title:    count + " workers need healing",
		BodyKey:  "notify_worker_failed_group_body",
		Body:     workers + ". Open lerd to heal.",
		Params: map[string]string{
			"count":   count,
			"workers": workers,
			"sites":   strings.Join(sites, ", "),
		},
		Tag:     "lerd-workers-group",
		URL:     "#sites",
		Data:    map[string]string{"count": count},
		Urgency: "high",
		TTL:     300,
	}
}
