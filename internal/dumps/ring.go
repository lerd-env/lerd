package dumps

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// DefaultCapacity is how many events the ring keeps when dumps.buffer is unset,
// matching config.DefaultDumpsBuffer. An event-heavy request emits thousands, so
// 3000 let one push the requests before it out within seconds.
const DefaultCapacity = 5000

// Ring is the debug buffer: the newest cap events, plus every event of the kept
// requests however old. It lives in SQLite rather than memory, since a single
// slow page can carry thousands of events and lerd-ui has to stay small; a
// reader decodes only what it asked for. Safe for concurrent use.
type Ring struct {
	mu  sync.Mutex
	db  *sql.DB
	cap int
	// onDisk is false for an in-memory ring, which a restart loses.
	onDisk bool
	// keep names the requests whose events survive past the newest cap: each
	// route's slowest, which the dashboard links to for days.
	keep map[string]bool
	// untrimmed counts appends since the last trim. Readers only look at the
	// newest cap anyway, so the delete runs in batches rather than per event.
	untrimmed int
}

// trimEvery is how many appends go by between trims.
const trimEvery = 256

const ringSchema = `
CREATE TABLE IF NOT EXISTS events (
  seq     INTEGER PRIMARY KEY,
  id      TEXT    NOT NULL,
  rid     TEXT    NOT NULL,
  reached TEXT    NOT NULL,
  site    TEXT    NOT NULL,
  branch  TEXT    NOT NULL,
  ctx     TEXT    NOT NULL,
  kind    TEXT    NOT NULL,
  test    INTEGER NOT NULL,
  nav     INTEGER NOT NULL,
  grp     TEXT    NOT NULL,
  route   TEXT    NOT NULL,
  worker  TEXT    NOT NULL,
  facet   TEXT    NOT NULL,
  hay     TEXT    NOT NULL,
  fp      TEXT    NOT NULL,
  ms      REAL    NOT NULL,
  job     TEXT    NOT NULL,
  kept    INTEGER NOT NULL DEFAULT 0,
  data    BLOB    NOT NULL,
  brief   BLOB    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_rid ON events(rid);
CREATE INDEX IF NOT EXISTS idx_events_reached ON events(reached);
CREATE INDEX IF NOT EXISTS idx_events_kept ON events(kept);
CREATE INDEX IF NOT EXISTS idx_events_site_kind ON events(site, kind);
CREATE INDEX IF NOT EXISTS idx_events_grp ON events(grp);
CREATE INDEX IF NOT EXISTS idx_events_job ON events(job);
CREATE INDEX IF NOT EXISTS idx_events_id ON events(id);`

// OpenRing opens (creating if needed) the ring stored at path, so the buffer
// survives a lerd-ui restart. Non-positive capacity means DefaultCapacity.
func OpenRing(path string, capacity int) (*Ring, error) {
	// The buffer is a cache of what PHP sent: losing its last writes to a crash
	// costs nothing, so commits skip the fsync.
	if path != ":memory:" {
		// Events carry SQL bindings and request payloads, so owner-only, and
		// SQLite gives its WAL and shared-memory files the same mode.
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, err
		}
		f.Close()
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, err
		}
	}
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(OFF)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: every call already holds mu, and an in-memory database
	// exists per connection, so a second one would see an empty ring.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(ringSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("debug events schema: %w", err)
	}
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Ring{db: db, cap: capacity, onDisk: path != ":memory:"}, nil
}

// NewRing returns a ring held in an in-memory database, for a process that does
// not need the buffer to outlive it.
func NewRing(capacity int) *Ring {
	r, err := OpenRing(":memory:", capacity)
	if err != nil {
		panic(fmt.Sprintf("in-memory debug buffer: %v", err))
	}
	return r
}

// Close releases the database.
func (r *Ring) Close() error { return r.db.Close() }

// Resize changes how many events the ring keeps, dropping the oldest that no
// longer fit unless a kept request ran them.
func (r *Ring) Resize(capacity int) {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cap = capacity
	r.trim()
}

// Append stores e, dropping the oldest event once the ring is full unless it
// belongs to a kept request.
func (r *Ring) Append(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	warnRing("storing an event", r.insert(e))
}

// insert stores e and trims once enough have landed. Callers hold mu.
func (r *Ring) insert(e Event) error {
	// A worker's row of a Laravel job has no payload, which is only readable
	// where the job was dispatched, so it takes the queued row's.
	job, hasPayload := jobUUID(e)
	if job != "" && !hasPayload {
		var queued []byte
		if r.db.QueryRow(`SELECT data FROM events WHERE job = ? ORDER BY seq LIMIT 1`, job).Scan(&queued) == nil {
			var q struct {
				Data struct {
					Payload json.RawMessage `json:"payload"`
				} `json:"data"`
			}
			if json.Unmarshal(queued, &q) == nil && len(q.Data.Payload) > 0 {
				e.Data = withPayload(e.Data, q.Data.Payload)
			}
		}
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	reached := e.reachedRID()
	var fp string
	var ms float64
	if q, ok := e.Query(); ok {
		fp, ms = normalizeSQL(q.SQL), q.TimeMS
	}
	_, err = r.db.Exec(
		`INSERT INTO events(id, rid, reached, site, branch, ctx, kind, test, nav, grp, route, worker, facet, hay, fp, ms, job, kept, data, brief)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.Ctx.RID, reached, e.Ctx.Site, e.Ctx.Branch, e.Ctx.Type, e.Kind, e.Ctx.Test, e.isPageView(),
		groupKey(e), routeOf(e), e.Ctx.Worker, facetOf(e), haystack(e), fp, ms, job,
		r.keep[e.Ctx.RID] || r.keep[reached], data, brief(e, data))
	if err != nil {
		return err
	}
	if r.untrimmed++; r.untrimmed >= trimEvery {
		r.trim()
	}
	return nil
}

// SetKeep replaces the set of requests whose events outlive the buffer, and
// lets go of the ones no longer in it.
func (r *Ring) SetKeep(keep map[string]bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keep = keep
	tx, err := r.db.Begin()
	if err != nil {
		warnRing("marking kept requests", err)
		return
	}
	_, err = tx.Exec(`UPDATE events SET kept = 0 WHERE kept = 1`)
	for rid := range keep {
		if err != nil {
			break
		}
		_, err = tx.Exec(`UPDATE events SET kept = 1 WHERE rid = ? OR reached = ?`, rid, rid)
	}
	if err != nil {
		tx.Rollback()
		warnRing("marking kept requests", err)
		return
	}
	warnRing("marking kept requests", tx.Commit())
	r.trim()
}

// trim deletes what fell out of the newest cap events and no kept request ran.
func (r *Ring) trim() {
	r.untrimmed = 0
	_, err := r.db.Exec(`DELETE FROM events WHERE kept = 0 AND seq < ?`, r.floor())
	warnRing("dropping old events", err)
}

// floor is the oldest seq still among the newest cap events, 0 while the ring
// holds fewer.
func (r *Ring) floor() int64 {
	var seq int64
	err := r.db.QueryRow(`SELECT seq FROM events ORDER BY seq DESC LIMIT 1 OFFSET ?`, r.cap-1).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0
	}
	warnRing("reading the buffer", err)
	return seq
}

// Snapshot returns the buffer, the newest cap events, oldest first.
func (r *Ring) Snapshot() []Event {
	return r.Filter(FilterOpts{})
}

// Len returns how many events the buffer holds, kept requests aside.
func (r *Ring) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int
	warnRing("counting events", r.db.QueryRow(`SELECT COUNT(*) FROM events WHERE seq >= ?`, r.floor()).Scan(&n))
	return n
}

// Cap returns the maximum number of events the buffer holds.
func (r *Ring) Cap() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cap
}

// Clear empties the ring, kept requests included.
func (r *Ring) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.db.Exec(`DELETE FROM events`)
	warnRing("clearing events", err)
}

// Remove drops every event drop matches, kept requests included.
func (r *Ring) Remove(drop func(Event) bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// First, or rows already out of the buffer would slide back into it once
	// the removed ones stop counting toward cap.
	r.trim()
	rows, err := r.db.Query(`SELECT seq, data FROM events`)
	if err != nil {
		warnRing("reading events", err)
		return
	}
	var doomed []int64
	for rows.Next() {
		var seq int64
		var data []byte
		var e Event
		if rows.Scan(&seq, &data) == nil && json.Unmarshal(data, &e) == nil && drop(e) {
			doomed = append(doomed, seq)
		}
	}
	rows.Close()
	tx, err := r.db.Begin()
	if err != nil {
		warnRing("removing events", err)
		return
	}
	for _, seq := range doomed {
		if _, err := tx.Exec(`DELETE FROM events WHERE seq = ?`, seq); err != nil {
			tx.Rollback()
			warnRing("removing events", err)
			return
		}
	}
	warnRing("removing events", tx.Commit())
}

// Import appends the events an older lerd-ui saved to a JSON file at path,
// oldest first, and deletes the file once every one is on disk. Until then it
// is their only copy, so a write that fails, or a ring held in memory, leaves
// it for the next start. A missing file is nothing to import.
func (r *Ring) Import(path string) error {
	if !r.onDisk {
		return nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var events []Event
	err = json.Unmarshal(b, &events)
	b = nil
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range events {
		if err := r.insert(e); err != nil {
			return fmt.Errorf("importing the old debug buffer: %w", err)
		}
	}
	return os.Remove(path)
}

// RequestIDs is the set of requests with at least one event in the ring: the
// ones a recent request's Inspect can open on something.
func (r *Ring) RequestIDs() map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]bool{}
	rows, err := r.db.Query(
		`SELECT rid FROM events WHERE seq >= ?1 OR kept = 1
		 UNION SELECT reached FROM events WHERE reached != '' AND (seq >= ?1 OR kept = 1)`, r.floor())
	if err != nil {
		warnRing("listing requests", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var rid string
		if rows.Scan(&rid) == nil && rid != "" {
			out[rid] = true
		}
	}
	return out
}

// FilterOpts narrows a Snapshot. Zero-value fields are ignored.
type FilterOpts struct {
	// Site exact-matches Ctx.Site when non-empty.
	Site string
	// Branch exact-matches Ctx.Branch when non-empty, isolating one git
	// worktree's events from the parent site they share a Site name with.
	Branch string
	// Ctx exact-matches Ctx.Type ("fpm" or "cli") when non-empty.
	Ctx string
	// Kind exact-matches Event.Kind when non-empty (e.g. "query", "dump").
	Kind string
	// RID keeps one request's events: those it ran, and a browser event that
	// names it as the request a fetch reached. A kept request is found even
	// after it left the buffer.
	RID string
	// SinceID drops events whose ID is lexicographically <= SinceID.
	SinceID string
	// Before keeps only events whose ID sorts before it, so a list pages back
	// from the oldest row it already shows.
	Before string
	// HideTests leaves out events captured inside a test run.
	HideTests bool
	// Route keeps one route as the timing view names it ("GET /users/:id").
	Route string
	// Limit caps the returned slice to the most recent N entries.
	// Zero or negative means no limit.
	Limit int
}

// Filter returns the events opts selects, oldest first.
func (r *Ring) Filter(opts FilterOpts) []Event {
	out := []Event{}
	r.eachRow(opts, func(data []byte) {
		var e Event
		if json.Unmarshal(data, &e) == nil {
			out = append(out, e)
		}
	})
	return out
}

// FilterJSON is Filter encoded as a JSON array, built from the stored rows as
// they are, so serving thousands of events costs one copy rather than a decode
// and an encode of each.
func (r *Ring) FilterJSON(opts FilterOpts) []byte {
	buf := []byte{'['}
	r.eachRow(opts, func(data []byte) {
		if len(buf) > 1 {
			buf = append(buf, ',')
		}
		buf = append(buf, data...)
	})
	return append(buf, ']')
}

// where is the condition opts selects on, and its arguments.
func (r *Ring) where(opts FilterOpts) (string, []any) {
	where := []string{"seq >= ?"}
	args := []any{r.floor()}
	if opts.RID != "" {
		where = []string{"(seq >= ? OR kept = 1)", "(rid = ? OR reached = ?)"}
		args = append(args, opts.RID, opts.RID)
	}
	for _, f := range []struct{ col, val string }{
		{"site", opts.Site}, {"branch", opts.Branch}, {"ctx", opts.Ctx}, {"kind", opts.Kind},
	} {
		if f.val != "" {
			where = append(where, f.col+" = ?")
			args = append(args, f.val)
		}
	}
	if opts.SinceID != "" {
		where = append(where, "id > ?")
		args = append(args, opts.SinceID)
	}
	if opts.Before != "" {
		where = append(where, "id < ?")
		args = append(args, opts.Before)
	}
	if opts.HideTests {
		where = append(where, "test = 0")
	}
	if opts.Route != "" {
		where = append(where, "route = ?")
		args = append(args, opts.Route)
	}
	return strings.Join(where, " AND "), args
}

// eachRow calls fn with the stored JSON of every event opts selects, oldest
// first. data is only valid during the call.
func (r *Ring) eachRow(opts FilterOpts, fn func(data []byte)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cond, args := r.where(opts)
	// The newest Limit, oldest first: find where they start, then read forward
	// on the primary key, so SQLite streams rows instead of sorting a copy.
	if opts.Limit > 0 {
		var from int64
		err := r.db.QueryRow(`SELECT seq FROM events WHERE `+cond+` ORDER BY seq DESC LIMIT 1 OFFSET ?`,
			append(args, opts.Limit-1)...).Scan(&from)
		if err == nil {
			cond += " AND seq >= ?"
			args = append(args, from)
		} else if !errors.Is(err, sql.ErrNoRows) {
			warnRing("reading events", err)
			return
		}
	}
	rows, err := r.db.Query(`SELECT data FROM events WHERE `+cond+` ORDER BY seq`, args...)
	if err != nil {
		warnRing("reading events", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var data sql.RawBytes
		if rows.Scan(&data) == nil {
			fn(data)
		}
	}
}

// Counts is how many events of each kind opts selects, page views aside: the
// lens bar's badges, counted where the events are rather than in a tab.
func (r *Ring) Counts(opts FilterOpts) map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	cond, args := r.where(opts)
	out := map[string]int{}
	rows, err := r.db.Query(`SELECT kind, COUNT(*) FROM events WHERE `+cond+` AND nav = 0 GROUP BY kind`, args...)
	if err != nil {
		warnRing("counting events", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var n int
		if rows.Scan(&kind, &n) == nil {
			out[kind] = n
		}
	}
	return out
}

// TestCount is how many test-run events opts would select with them shown,
// for the hint that some are hidden.
func (r *Ring) TestCount(opts FilterOpts) int {
	opts.HideTests = false
	r.mu.Lock()
	defer r.mu.Unlock()
	cond, args := r.where(opts)
	var n int
	warnRing("counting events", r.db.QueryRow(`SELECT COUNT(*) FROM events WHERE `+cond+` AND test = 1 AND nav = 0`, args...).Scan(&n))
	return n
}

// Sites lists every site with an event in the buffer, for the site filter.
func (r *Ring) Sites() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []string{}
	rows, err := r.db.Query(`SELECT DISTINCT site FROM events WHERE seq >= ? ORDER BY site`, r.floor())
	if err != nil {
		warnRing("listing sites", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var site string
		if rows.Scan(&site) == nil {
			out = append(out, site)
		}
	}
	return out
}

// warnRing reports a failed database call on lerd-ui's log. Capture carries on
// for the next event, so one failed write never stops the stream.
func warnRing(what string, err error) {
	if err != nil {
		fmt.Printf("[WARN] debug buffer, %s: %v\n", what, err)
	}
}
