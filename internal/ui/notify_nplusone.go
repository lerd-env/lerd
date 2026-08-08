package ui

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
	"github.com/geodro/lerd/internal/push"
	"github.com/geodro/lerd/internal/reqstats"
)

// nPlusOneThreshold is the number of structurally-identical queries within one
// request/invocation that trips the N+1 warning. Matches the dashboard's
// NPLUSONE_AT so the notification and the in-UI badge agree.
const nPlusOneThreshold = 3

// maxTrackedRequests bounds the per-request fingerprint state so a long-lived
// lerd-ui can't accumulate one map per request forever.
const maxTrackedRequests = 512

var (
	reSQLSingle = regexp.MustCompile(`'(?:[^'\\]|\\.)*'`)
	reSQLDouble = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	reSQLNum    = regexp.MustCompile(`\b\d+\b`)
	reSQLWS     = regexp.MustCompile(`\s+`)
)

// normalizeSQL collapses literal values so structurally-identical queries share
// a fingerprint. Mirrors the TS normalizeSql in stores/queries.ts.
func normalizeSQL(sql string) string {
	sql = reSQLSingle.ReplaceAllString(sql, "?")
	sql = reSQLDouble.ReplaceAllString(sql, "?")
	sql = reSQLNum.ReplaceAllString(sql, "?")
	sql = reSQLWS.ReplaceAllString(sql, " ")
	return strings.ToLower(strings.TrimSpace(sql))
}

// nPlusOneTracker watches query events and reports the first time a request's
// query shape repeats past the threshold. It fires at most once per route (FPM
// method+path with ids masked) or per worker command, so it warns without
// nagging on every subsequent hit of the same endpoint within a session.
type nPlusOneTracker struct {
	mu     sync.Mutex
	perReq map[string]map[string]int // rid -> fingerprint -> count
	order  []string                  // rid insertion order, for eviction
	warned map[string]bool           // route key -> already warned
	warns  map[string]bool           // site -> its framework wants the warning
}

func newNPlusOneTracker() *nPlusOneTracker {
	return &nPlusOneTracker{
		perReq: map[string]map[string]int{},
		warned: map[string]bool{},
		warns:  map[string]bool{},
	}
}

// siteWarns reports whether the site's framework wants repeated-query warnings.
// A content management system issues the repeats from its own entity, config
// and cache layers, so the warning names a loop inside the framework rather
// than anything the developer wrote, and its definition says so.
//
// Resolved once per site: this is asked of every query event, and the answer
// changes about as often as a site changes framework.
func (t *nPlusOneTracker) siteWarns(site string) bool {
	if site == "" {
		return true
	}
	if v, ok := t.warns[site]; ok {
		return v
	}
	warns := true
	if s, err := config.FindSite(site); err == nil && s != nil {
		if fw, ok := config.GetFrameworkForDir(s.Framework, s.Path); ok {
			warns = fw.WarnsNPlusOne()
		}
	}
	t.warns[site] = warns
	return warns
}

// routeKeyForQuery collapses a query event to the "route or script" the warning
// is deduped on: the worker command, the CLI invocation, or the site plus the
// request normalized through the same reqstats route key the timing snapshot
// uses, so /users/1 and /users/2 share a key and the two detectors bucket
// identically. Without the command arm every console invocation would share one
// key and only the first artisan command of a session could ever warn.
func routeKeyForQuery(ev dumps.Event) string {
	if ev.Ctx.Worker != "" {
		return "worker:" + ev.Ctx.Worker
	}
	if ev.Ctx.Request == "" && ev.Ctx.Command != "" {
		return "cli:" + ev.Ctx.Command
	}
	method, path, _ := strings.Cut(ev.Ctx.Request, " ")
	return ev.Ctx.Site + " " + reqstats.NormalizeRoute(method, path)
}

// observe records a query event and returns a notification the first time a
// fingerprint in its request crosses the threshold for an un-warned route.
func (t *nPlusOneTracker) observe(ev dumps.Event) *push.Notification {
	if ev.Ctx.RID == "" {
		return nil // no request boundary to group within
	}
	q, ok := ev.Query()
	if !ok || q.SQL == "" {
		return nil
	}
	// A static asset is not a route anyone can optimise, and a framework that
	// builds its aggregates through PHP runs queries serving one. Worse, each
	// aggregate carries its own hash, so every request is a route the tracker
	// has not warned about yet and the warnings arrive one per asset. The
	// timing view already drops them by the same test.
	if _, path, found := strings.Cut(ev.Ctx.Request, " "); found && reqstats.IsStaticAsset(path) {
		return nil
	}
	route := routeKeyForQuery(ev)

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.warned[route] || !t.siteWarns(ev.Ctx.Site) {
		return nil
	}
	m := t.perReq[ev.Ctx.RID]
	if m == nil {
		m = map[string]int{}
		t.perReq[ev.Ctx.RID] = m
		t.order = append(t.order, ev.Ctx.RID)
		t.evict()
	}
	fp := normalizeSQL(q.SQL)
	m[fp]++
	if m[fp] < nPlusOneThreshold {
		return nil
	}
	t.warned[route] = true
	delete(t.perReq, ev.Ctx.RID)
	n := notificationForNPlusOne(ev, m[fp])
	return &n
}

func (t *nPlusOneTracker) evict() {
	for len(t.order) > maxTrackedRequests {
		oldest := t.order[0]
		t.order = t.order[1:]
		delete(t.perReq, oldest)
	}
}

// whereForQuery names the run a warning came from, most specific first: the
// worker command, the CLI invocation, or the request route. The query's own
// file:line stays out of the body; the dumps lens carries it on the event.
func whereForQuery(ev dumps.Event) string {
	for _, s := range []string{ev.Ctx.Worker, ev.Ctx.Command, ev.Ctx.Request} {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

func notificationForNPlusOne(ev dumps.Event, count int) push.Notification {
	site := ev.Ctx.Site
	if site == "" {
		site = "(unknown site)"
	}
	body := fmt.Sprintf("Ran a similar query %d× in one request", count)
	if where := whereForQuery(ev); where != "" {
		body = fmt.Sprintf("%s ran a similar query %d×", where, count)
	}
	return push.Notification{
		Kind:  "nplusone",
		Title: "Possible N+1 query on " + site,
		Body:  body,
		Tag:   "lerd-nplusone-" + routeKeyForQuery(ev),
		URL:   debugRouteForContext(ev.Ctx),
		Data: map[string]string{
			"site":    ev.Ctx.Site,
			"worker":  ev.Ctx.Worker,
			"command": ev.Ctx.Command,
		},
		Urgency: "normal",
		TTL:     120,
	}
}
