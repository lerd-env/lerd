package reqstats

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// SiteResolver maps a request host to the owning site name, returning ok=false
// for a host that belongs to no registered site. Mirrors the idle resolver so
// the watcher can pass the same function.
type SiteResolver func(host string) (site string, ok bool)

// Tuning defaults. The window bounds memory per route; the sample floor keeps a
// route off the slow list until it has been seen enough to trust its median; the
// factor is how many times its own baseline a route must exceed to count as slow.
const (
	defaultWindow    = 200
	defaultMaxRoutes = 500
	defaultMinSample = 5
	defaultFactor    = 3.0
	slowListLimit    = 10
	// slowAbsoluteMillis flags a route whose p95 is slow in absolute terms even
	// below the sample floor: a full second is worth surfacing from a single hit,
	// since a local dev rarely repeats a slow page enough to clear defaultMinSample.
	slowAbsoluteMillis = 1000.0
	// relSlowFloorMillis is the p95 a route must clear before the relative
	// (Nx-the-baseline) rule flags it, so a fast route isn't called slow when a
	// frequent fast endpoint drags the baseline toward zero.
	relSlowFloorMillis = 150.0
)

// recentWindow bounds the snapshot to recently-seen samples, so a route that was
// slow but has been fixed (or simply left alone) ages out instead of lingering on
// stale outliers, which matters most for low-volume routes the count-based buffer
// would otherwise hold onto until a couple hundred fresh hits overwrite them.
const recentWindow = 10 * time.Minute

// RouteStat is one flagged route in a site snapshot. The representative time is
// the route's p95, not its median: a route that is only sometimes slow (a mix of
// fast redirects and slow renders under one path) hides in the median but shows
// in the tail, which is the "this request takes a second" the user actually feels.
type RouteStat struct {
	Route     string  `json:"route"`
	Method    string  `json:"method"`
	Example   string  `json:"example"` // a concrete path last seen for this route, openable in a browser
	P50Millis float64 `json:"p50_millis,omitempty"`
	P95Millis float64 `json:"p95_millis"`
	// RecentP95Millis is the p95 over only the route's most recent samples, so a
	// route that was slow but has since been fixed reads as fast again even while
	// its old slow samples still sit in the window's overall p95.
	RecentP95Millis float64 `json:"recent_p95_millis,omitempty"`
	Multiplier      float64 `json:"multiplier"`
	Samples         int     `json:"samples"`
}

// SiteStats is the per-site view the UI renders: the typical response time and
// the routes running well above their own baseline, slowest first.
type SiteStats struct {
	Site         string      `json:"site"`
	MedianMillis float64     `json:"median_millis"`
	Samples      int         `json:"samples"`
	Slow         []RouteStat `json:"slow"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// Aggregator holds rolling per-site, per-route timing windows. Safe for
// concurrent use: the access-feed reader writes while the UI reads.
type Aggregator struct {
	mu      sync.Mutex
	sites   map[string]*siteAgg
	resolve SiteResolver
	now     func() time.Time // injectable clock, for deterministic decay in tests
}

type siteAgg struct {
	overall *ring
	routes  map[string]*routeAgg
	updated time.Time
}

// routeAgg is one route's rolling window plus the bits needed to open it: its
// HTTP method and the most recent concrete path seen (query stripped).
type routeAgg struct {
	times   *ring
	method  string
	example string
	lastAt  int64 // UnixNano of the most recent sample, for stale-route eviction
}

// New returns an aggregator that maps hosts to sites via resolve. A nil resolver
// drops every record.
func New(resolve SiteResolver) *Aggregator {
	return &Aggregator{sites: map[string]*siteAgg{}, resolve: resolve, now: time.Now}
}

// Record ingests one access record, resolving its host to a site and appending
// the request time to that site's overall and per-route windows. Records for an
// unknown host, or once a site is at its route cap for a new route, are dropped.
func (a *Aggregator) Record(r AccessRecord) {
	if a.resolve == nil {
		return
	}
	site, ok := a.resolve(r.Host)
	if !ok {
		return
	}
	route := NormalizeRoute(r.Method, r.URI)
	ms := r.SecondsToMillis()

	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	at := now.UnixNano()
	sa := a.sites[site]
	if sa == nil {
		sa = &siteAgg{overall: newRing(defaultWindow), routes: map[string]*routeAgg{}}
		a.sites[site] = sa
	}
	sa.overall.add(ms, at)
	rr := sa.routes[route]
	if rr == nil {
		if len(sa.routes) >= defaultMaxRoutes {
			// At the cap: reclaim slots held by routes that have gone quiet past
			// the recent window before dropping this one, so a long-lived site
			// that has seen many distinct paths keeps recording live traffic
			// instead of wedging on dead routes.
			sa.evictStaleRoutes(now.Add(-recentWindow).UnixNano())
			if len(sa.routes) >= defaultMaxRoutes {
				return
			}
		}
		rr = &routeAgg{times: newRing(defaultWindow), method: strings.ToUpper(strings.TrimSpace(r.Method))}
		sa.routes[route] = rr
	}
	rr.times.add(ms, at)
	rr.lastAt = at
	rr.example = StripQueryFragment(r.URI)
	sa.updated = now
}

// evictStaleRoutes deletes routes whose most recent sample is older than cutoff.
// A route with no recent samples contributes nothing to a snapshot anyway, so
// freeing its slot is loss-free and keeps the route cap counting live routes.
func (sa *siteAgg) evictStaleRoutes(cutoff int64) {
	for route, r := range sa.routes {
		if r.lastAt < cutoff {
			delete(sa.routes, route)
		}
	}
}

// SiteSnapshot returns the current view for one site, ok=false when the site has
// no recorded traffic.
func (a *Aggregator) SiteSnapshot(site string) (SiteStats, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	sa := a.sites[site]
	if sa == nil {
		return SiteStats{}, false
	}
	return a.snapshotLocked(site, sa), true
}

// Snapshot returns a view for every site with recorded traffic.
func (a *Aggregator) Snapshot() []SiteStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]SiteStats, 0, len(a.sites))
	for site, sa := range a.sites {
		out = append(out, a.snapshotLocked(site, sa))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Site < out[j].Site })
	return out
}

func (a *Aggregator) snapshotLocked(site string, sa *siteAgg) SiteStats {
	cutoff := a.now().Add(-recentWindow).UnixNano()
	base := median(sa.overall.values(cutoff))
	st := SiteStats{
		Site:         site,
		MedianMillis: round1(base),
		Samples:      len(sa.overall.values(cutoff)),
		Slow:         []RouteStat{},
		UpdatedAt:    sa.updated,
	}
	for route, r := range sa.routes {
		vals := r.times.values(cutoff)
		if len(vals) == 0 {
			continue // no recent samples: the route has gone quiet, drop it
		}
		// Flag on the route's tail (p95). Relatively slow: p95 >= factor x the site
		// median, once it has enough samples to trust. Absolutely slow: p95 over a
		// full second, surfaced even from a single hit so a genuinely broken route a
		// dev triggers once doesn't hide under the sample floor.
		tail := percentile(vals, 95)
		relSlow := len(vals) >= defaultMinSample && base > 0 && tail >= relSlowFloorMillis && tail/base >= defaultFactor
		absSlow := tail >= slowAbsoluteMillis
		if !relSlow && !absSlow {
			continue
		}
		mult := 0.0
		if base > 0 {
			mult = tail / base
		}
		st.Slow = append(st.Slow, RouteStat{
			Route:      route,
			Method:     r.method,
			Example:    r.example,
			P95Millis:  round1(tail),
			Multiplier: round1(mult),
			Samples:    len(vals),
		})
	}
	sort.Slice(st.Slow, func(i, j int) bool { return st.Slow[i].Multiplier > st.Slow[j].Multiplier })
	if len(st.Slow) > slowListLimit {
		st.Slow = st.Slow[:slowListLimit]
	}
	return st
}

// median returns the median of vals without mutating the input.
func median(vals []float64) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}

// percentile returns the p-th percentile of vals (nearest-rank) without mutating
// the input. With few samples this tends toward the max, which is the intended
// "worst recent" behaviour for a tail metric.
func percentile(vals []float64, p float64) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	rank := int(math.Ceil(p / 100 * float64(n)))
	if rank < 1 {
		rank = 1
	}
	if rank > n {
		rank = n
	}
	return cp[rank-1]
}

func round1(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }
