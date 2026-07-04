package ui

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

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
}

func newNPlusOneTracker() *nPlusOneTracker {
	return &nPlusOneTracker{
		perReq: map[string]map[string]int{},
		warned: map[string]bool{},
	}
}

// routeKeyForQuery collapses a query event to the "route or script" the warning
// is deduped on: the worker command, or the site plus the request normalized
// through the same reqstats route key the timing snapshot uses, so /users/1 and
// /users/2 share a key and the two detectors bucket identically.
func routeKeyForQuery(ev dumps.Event) string {
	if ev.Ctx.Worker != "" {
		return "worker:" + ev.Ctx.Worker
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
	route := routeKeyForQuery(ev)

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.warned[route] {
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

func notificationForNPlusOne(ev dumps.Event, count int) push.Notification {
	site := ev.Ctx.Site
	if site == "" {
		site = "(unknown site)"
	}
	// Secondary context: the worker command or the request route.
	where := ev.Ctx.Worker
	if where == "" {
		where = strings.TrimSpace(ev.Ctx.Request)
	}
	body := fmt.Sprintf("Ran a similar query %d× in one request", count)
	if where != "" {
		body = fmt.Sprintf("%s ran a similar query %d×", where, count)
	}
	url := "#system/dump-bridge"
	if ev.Ctx.Site != "" {
		url = "#sites/" + siteDomainForRoute(ev.Ctx.Site) + "/dumps"
	}
	return push.Notification{
		Kind:  "nplusone",
		Title: "Possible N+1 query on " + site,
		Body:  body,
		Tag:   "lerd-nplusone-" + routeKeyForQuery(ev),
		URL:   url,
		Data: map[string]string{
			"site":   ev.Ctx.Site,
			"worker": ev.Ctx.Worker,
		},
		Urgency: "normal",
		TTL:     120,
	}
}
