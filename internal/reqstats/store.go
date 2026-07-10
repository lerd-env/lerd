package reqstats

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// RecordFrom builds a store record from a parsed access record for a resolved
// site, normalizing the route the same way the live aggregator does so the two
// views agree on route identity.
func RecordFrom(r AccessRecord, site string, at time.Time) Record {
	return Record{
		At:     at,
		Site:   site,
		Route:  NormalizeRoute(r.Method, r.URI),
		Method: strings.ToUpper(strings.TrimSpace(r.Method)),
		Status: r.Status,
		Millis: r.SecondsToMillis(),
		URI:    StripQueryFragment(r.URI),
	}
}

// DefaultColdGap is the fallback idle gap after which a request is treated as a
// cold start, used when the idle-suspend timeout can't be read. The watcher
// prefers that configured timeout.
const DefaultColdGap = 30 * time.Minute

// IsColdStart reports whether a request at now is a cold start: the site has been
// seen before and sat idle at least gap since. The first request ever seen for a
// site isn't a cold start, since there's no prior time to prove it was idle; the
// watcher seeds the last-seen clock from the durable store on startup so a wake
// right after a daemon restart still counts against a real prior time.
func IsColdStart(last time.Time, seen bool, now time.Time, gap time.Duration) bool {
	return seen && gap > 0 && now.Sub(last) >= gap
}

// latencyEdges is the fixed ladder the response-time distribution buckets into.
// Each edge is an exclusive upper bound in milliseconds; a final open-ended
// bucket (UpperMillis 0) catches everything above the last edge.
var latencyEdges = []float64{25, 50, 100, 250, 500, 1000}

// recentRouteWindow is how many of a route's most recent samples feed its recent
// p95, the recency-aware figure the slowest-routes list ranks by so a fixed route
// falls off once newer, faster requests arrive.
const recentRouteWindow = 20

// Store is the durable SQLite record of requests. The watcher (the only process
// on the nginx access feed) writes to it; lerd-ui opens it read-only to build the
// request-timing analytics view over any window. Pure-Go driver, so the CGO-free
// build is unaffected.
type Store struct {
	db *sql.DB
}

// Record is one persisted request. Route is the normalized template (e.g.
// "GET /products/:id"); URI is the concrete path last seen, so a route stays
// openable and the recent list shows real paths.
type Record struct {
	At     time.Time
	Site   string
	Route  string
	Method string
	Status int
	Millis float64
	URI    string
	// Cold marks the first request after the site had gone idle: a cold start
	// (suspended workers waking, cold caches) whose inflated time would skew the
	// timing view, so it's kept out of the percentiles but still counted.
	Cold bool
}

// LatencyBucket is one bar of the response-time histogram. UpperMillis is the
// exclusive upper bound; 0 marks the open-ended top bucket.
type LatencyBucket struct {
	UpperMillis float64 `json:"upper_millis"`
	Count       int     `json:"count"`
}

// StatusCounts is the response-status breakdown, used for the error rate.
type StatusCounts struct {
	C2xx int `json:"c2xx"`
	C3xx int `json:"c3xx"`
	C4xx int `json:"c4xx"`
	C5xx int `json:"c5xx"`
}

// ThroughputPoint is the request count in one minute bucket, keyed by the
// bucket's start as unix milliseconds.
type ThroughputPoint struct {
	AtMillis int64 `json:"at_millis"`
	Count    int   `json:"count"`
}

// Analytics is the full request-timing view for one site over a window.
type Analytics struct {
	Site         string            `json:"site"`
	Samples      int               `json:"samples"`
	ColdStarts   int               `json:"cold_starts"` // excluded from timing, still counted
	MedianMillis float64           `json:"median_millis"`
	P95Millis    float64           `json:"p95_millis"`
	Status       StatusCounts      `json:"status"`
	Distribution []LatencyBucket   `json:"distribution"`
	Throughput   []ThroughputPoint `json:"throughput"`
	Routes       []RouteStat       `json:"routes"`
}

// OpenStore opens (creating if needed) the SQLite store at path with WAL enabled
// so the watcher can write while lerd-ui reads across processes.
func OpenStore(path string) (*Store, error) {
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	// Migrate stores created before the cold column existed. The ALTER errors with
	// "duplicate column name" on an up-to-date store, which is the expected no-op.
	if _, err := db.Exec(`ALTER TABLE requests ADD COLUMN cold INTEGER NOT NULL DEFAULT 0`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		db.Close()
		return nil, fmt.Errorf("migrate cold column: %w", err)
	}
	return &Store{db: db}, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS requests (
  at_ms  INTEGER NOT NULL,
  site   TEXT    NOT NULL,
  route  TEXT    NOT NULL,
  method TEXT    NOT NULL,
  status INTEGER NOT NULL,
  ms     REAL    NOT NULL,
  uri    TEXT    NOT NULL,
  cold   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_requests_site_at ON requests(site, at_ms);`

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// Insert writes a batch of records in one transaction, so the watcher can buffer
// a tick's worth of requests and flush them together rather than per request.
func (s *Store) Insert(recs []Record) error {
	if len(recs) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO requests(at_ms, site, route, method, status, ms, uri, cold) VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r := range recs {
		if _, err := stmt.Exec(r.At.UnixMilli(), r.Site, r.Route, r.Method, r.Status, r.Millis, r.URI, boolToInt(r.Cold)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// Prune deletes rows older than before, returning how many were removed, so the
// watcher can bound the store to a retention window.
func (s *Store) Prune(before time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM requests WHERE at_ms < ?`, before.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// LastSeenBySite returns the most recent request time for every site in the
// store, so the watcher can seed its cold-start clock on startup. Without it a
// daemon restart forgets the last request and judges the next wake as warm,
// letting the cold boot's inflated time dominate the route p95.
func (s *Store) LastSeenBySite() (map[string]time.Time, error) {
	rows, err := s.db.Query(`SELECT site, MAX(at_ms) FROM requests GROUP BY site`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var site string
		var atMs int64
		if err := rows.Scan(&site, &atMs); err != nil {
			return nil, err
		}
		out[site] = time.UnixMilli(atMs)
	}
	return out, rows.Err()
}

// Recent returns the newest limit requests for a site, newest first.
func (s *Store) Recent(site string, limit int) ([]Record, error) {
	// Over-fetch and drop static assets in Go, so a burst of asset requests can't
	// crowd real requests out of the list; the scan is capped so it stays cheap.
	rows, err := s.db.Query(
		`SELECT at_ms, route, method, status, ms, uri, cold FROM requests WHERE site = ? ORDER BY at_ms DESC LIMIT ?`,
		site, limit*20+100)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		if len(out) >= limit {
			break
		}
		var atMs int64
		var cold int
		var r = Record{Site: site}
		if err := rows.Scan(&atMs, &r.Route, &r.Method, &r.Status, &r.Millis, &r.URI, &cold); err != nil {
			return nil, err
		}
		if !IsAppRequest(r.Status, r.URI, r.Millis) {
			continue
		}
		r.At = time.UnixMilli(atMs)
		r.Cold = cold != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// SiteAnalytics aggregates every request for a site in [since, until) into the
// full analytics view. Percentiles are computed in Go from the window's rows,
// reusing the same helpers as the live aggregator, since SQLite has no native
// percentile and local request volumes are small.
func (s *Store) SiteAnalytics(site string, since, until time.Time) (Analytics, error) {
	rows, err := s.db.Query(
		`SELECT at_ms, route, method, status, ms, uri, cold FROM requests
		 WHERE site = ? AND at_ms >= ? AND at_ms < ? ORDER BY at_ms ASC`,
		site, since.UnixMilli(), until.UnixMilli())
	if err != nil {
		return Analytics{}, err
	}
	defer rows.Close()

	a := Analytics{Site: site, Distribution: emptyBuckets(), Throughput: []ThroughputPoint{}, Routes: []RouteStat{}}
	var warm []float64
	type agg struct {
		method, example string
		warm            []float64
		total           int
	}
	routes := map[string]*agg{}
	minutes := map[int64]int{}

	for rows.Next() {
		var atMs int64
		var route, method, uri string
		var status, cold int
		var ms float64
		if err := rows.Scan(&atMs, &route, &method, &status, &ms, &uri, &cold); err != nil {
			return Analytics{}, err
		}
		// New ones aren't recorded; filtering on read also drops any already stored
		// before the ingest filter existed.
		if !IsAppRequest(status, uri, ms) {
			continue
		}
		// Cold starts count toward the total, status, and throughput, but are kept
		// out of every timing figure (site and per-route percentiles, distribution)
		// so a wake never makes a route look slow.
		a.Samples++
		classify(&a.Status, status)
		minutes[atMs/60000*60000]++
		r := routes[route]
		if r == nil {
			r = &agg{method: method}
			routes[route] = r
		}
		r.total++
		r.example = uri
		if cold != 0 {
			a.ColdStarts++
			continue
		}
		warm = append(warm, ms)
		a.Distribution[bucketIndex(ms)].Count++
		r.warm = append(r.warm, ms)
	}
	if err := rows.Err(); err != nil {
		return Analytics{}, err
	}

	a.MedianMillis = round1(median(warm))
	a.P95Millis = round1(percentile(warm, 95))
	for route, r := range routes {
		// r.warm is in time order (rows come back oldest first), so the tail slice is
		// the route's most recent samples. A route seen only as cold starts has no
		// warm samples; fall back to a zero timing rather than dropping the row.
		recent := r.warm
		if len(recent) > recentRouteWindow {
			recent = recent[len(recent)-recentRouteWindow:]
		}
		a.Routes = append(a.Routes, RouteStat{
			Route:           route,
			Method:          r.method,
			Example:         r.example,
			P50Millis:       round1(median(r.warm)),
			P95Millis:       round1(percentile(r.warm, 95)),
			RecentP95Millis: round1(percentile(recent, 95)),
			Samples:         r.total,
		})
	}
	sort.Slice(a.Routes, func(i, j int) bool {
		if a.Routes[i].Samples != a.Routes[j].Samples {
			return a.Routes[i].Samples > a.Routes[j].Samples
		}
		return a.Routes[i].Route < a.Routes[j].Route
	})
	for at, count := range minutes {
		a.Throughput = append(a.Throughput, ThroughputPoint{AtMillis: at, Count: count})
	}
	sort.Slice(a.Throughput, func(i, j int) bool { return a.Throughput[i].AtMillis < a.Throughput[j].AtMillis })
	return a, nil
}

// emptyBuckets returns a zeroed distribution ladder: one bucket per edge plus the
// open-ended top bucket.
func emptyBuckets() []LatencyBucket {
	b := make([]LatencyBucket, len(latencyEdges)+1)
	for i, e := range latencyEdges {
		b[i].UpperMillis = e
	}
	return b
}

// bucketIndex maps a response time to its distribution bucket: the first edge it
// falls under, or the open-ended top bucket.
func bucketIndex(ms float64) int {
	for i, e := range latencyEdges {
		if ms < e {
			return i
		}
	}
	return len(latencyEdges)
}

// boolToInt maps a bool to the 0/1 SQLite stores for the cold flag.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// classify increments the status-class counter for one response code.
func classify(s *StatusCounts, status int) {
	switch status / 100 {
	case 2:
		s.C2xx++
	case 3:
		s.C3xx++
	case 4:
		s.C4xx++
	case 5:
		s.C5xx++
	}
}
