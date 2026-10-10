package dumps

import (
	"encoding/json"
	"strings"
)

// The Debug lenses read their rows from here: events of one kind grouped by
// request, searched, narrowed and paged in SQL, so a tab holds one page.

// SlowMS tags a query at or above it as slow, Telescope's default threshold.
const SlowMS = 100

// A SQL fingerprint seen this often in one request marks its rows duplicates,
// and this many escalates the request to an N+1 warning.
const (
	duplicateAt = 2
	nPlusOneAt  = 3
)

// GroupOpts narrows a lens. FilterOpts carries the scope (site, branch, kind,
// request, test runs); the rest are the lens's own controls.
type GroupOpts struct {
	FilterOpts
	// Search matches the lens's text: a query's SQL, a dump's output, any
	// other kind's payload, and every event's request, route and id.
	Search string
	// Worker keeps one queue or scheduler command's events.
	Worker string
	// HideWorkers leaves out what worker processes emitted, except jobs, which
	// are the only sign a queue is being drained.
	HideWorkers bool
	// Facet keeps one job status, log level or browser event type.
	Facet string
	// Before is a page's Next, to read the groups older than that page.
	Before int64
	// Limit is how many groups a page holds, Rows how many rows each carries.
	Limit int
	Rows  int
}

// Group is one request in a lens, its newest Rows rows first.
type Group struct {
	Key       string  `json:"key"`
	Count     int     `json:"count"`
	TotalMS   float64 `json:"total_ms"`
	SlowCount int     `json:"slow_count"`
	NPlusOne  bool    `json:"n_plus_one"`
	Rows      []Row   `json:"rows"`
	last      int64
}

// Row is one event and, for a query, how often its fingerprint ran in the
// request (1 when unique). Seq is its place in the buffer, which reading more
// of the group pages back from.
type Row struct {
	Event json.RawMessage `json:"event"`
	Dup   int             `json:"dup"`
	Seq   int64           `json:"seq"`
}

// GroupPage is one page of a lens. Next reads the page after it, 0 at the end;
// Total is every event the lens matches, across all its pages.
type GroupPage struct {
	Groups []Group `json:"groups"`
	Next   int64   `json:"next"`
	Total  int     `json:"total"`
}

func (r *Ring) groupWhere(opts GroupOpts) (string, []any) {
	cond, args := r.where(opts.FilterOpts)
	add := func(c string, a ...any) {
		cond += " AND " + c
		args = append(args, a...)
	}
	if opts.Search != "" {
		add("instr(hay, ?) > 0", strings.ToLower(opts.Search))
	}
	if opts.Worker != "" {
		add("worker = ?", opts.Worker)
	}
	if opts.HideWorkers {
		add("(worker = '' OR kind = ?)", KindJob)
	}
	if opts.Facet != "" {
		add("facet = ?", opts.Facet)
	}
	return cond + " AND nav = 0", args
}

// Groups returns one page of opts's lens: the requests with the latest
// activity first, each with its count, query time and N+1 flag.
func (r *Ring) Groups(opts GroupOpts) GroupPage {
	r.mu.Lock()
	defer r.mu.Unlock()
	page := GroupPage{Groups: []Group{}}
	cond, args := r.groupWhere(opts)
	warnRing("counting a lens", r.db.QueryRow(`SELECT COUNT(*) FROM events WHERE `+cond, args...).Scan(&page.Total))
	q := `SELECT grp, MAX(seq) AS last, COUNT(*), SUM(ms), SUM(ms >= ?) FROM events WHERE ` + cond + ` GROUP BY grp`
	args = append([]any{SlowMS}, args...)
	if opts.Before > 0 {
		q += ` HAVING last < ?`
		args = append(args, opts.Before)
	}
	q += ` ORDER BY last DESC LIMIT ?`
	args = append(args, opts.Limit+1)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		warnRing("grouping events", err)
		return page
	}
	for rows.Next() {
		var g Group
		if rows.Scan(&g.Key, &g.last, &g.Count, &g.TotalMS, &g.SlowCount) == nil {
			page.Groups = append(page.Groups, g)
		}
	}
	rows.Close()
	if len(page.Groups) > opts.Limit {
		page.Groups = page.Groups[:opts.Limit]
		page.Next = page.Groups[len(page.Groups)-1].last
	}
	for i := range page.Groups {
		g := &page.Groups[i]
		dups := r.fingerprints(opts, g.Key)
		for _, n := range dups {
			if n >= nPlusOneAt {
				g.NPlusOne = true
			}
		}
		g.Rows = r.rows(opts, g.Key, 0, dups)
	}
	return page
}

// GroupRows reads more of one group's rows, the ones older than the row at
// before, newest first. Paging from a row rather than an offset keeps events
// that land in the group meanwhile from handing back a row already shown.
func (r *Ring) GroupRows(opts GroupOpts, key string, before int64) []Row {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rows(opts, key, before, r.fingerprints(opts, key))
}

// fingerprints counts how often each query shape ran in one group.
func (r *Ring) fingerprints(opts GroupOpts, key string) map[string]int {
	out := map[string]int{}
	cond, args := r.groupWhere(opts)
	rows, err := r.db.Query(`SELECT fp, COUNT(*) FROM events WHERE `+cond+` AND grp = ? AND fp != '' GROUP BY fp`, append(args, key)...)
	if err != nil {
		warnRing("counting query shapes", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var fp string
		var n int
		if rows.Scan(&fp, &n) == nil {
			out[fp] = n
		}
	}
	return out
}

func (r *Ring) rows(opts GroupOpts, key string, before int64, dups map[string]int) []Row {
	out := []Row{}
	cond, args := r.groupWhere(opts)
	cond += " AND grp = ?"
	args = append(args, key)
	if before > 0 {
		cond += " AND seq < ?"
		args = append(args, before)
	}
	rows, err := r.db.Query(`SELECT seq, brief, fp FROM events WHERE `+cond+` ORDER BY seq DESC LIMIT ?`, append(args, opts.Rows)...)
	if err != nil {
		warnRing("reading a group", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var seq int64
		var data []byte
		var fp string
		if rows.Scan(&seq, &data, &fp) != nil {
			continue
		}
		row := Row{Event: data, Dup: 1, Seq: seq}
		if n := dups[fp]; fp != "" && n > 1 {
			row.Dup = n
		}
		out = append(out, row)
	}
	return out
}

// Event is one whole event by id, for a row a lens opens.
func (r *Ring) Event(id string) (json.RawMessage, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var data []byte
	if err := r.db.QueryRow(`SELECT data FROM events WHERE id = ? ORDER BY seq DESC LIMIT 1`, id).Scan(&data); err != nil {
		return nil, false
	}
	return data, true
}

// FacetValues lists the facet values opts's kind has in its scope: a job's
// statuses, a log's levels, a message's channels, a browser event's types.
func (r *Ring) FacetValues(opts FilterOpts) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	cond, args := r.where(opts)
	out := []string{}
	rows, err := r.db.Query(`SELECT DISTINCT facet FROM events WHERE `+cond+` AND facet != '' ORDER BY facet`, args...)
	if err != nil {
		warnRing("listing facets", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if rows.Scan(&v) == nil {
			out = append(out, v)
		}
	}
	return out
}

// Workers lists the worker commands with events in opts's scope, for the lens
// command filter.
func (r *Ring) Workers(opts FilterOpts) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	cond, args := r.where(opts)
	out := []string{}
	rows, err := r.db.Query(`SELECT DISTINCT worker FROM events WHERE `+cond+` AND worker != '' ORDER BY worker`, args...)
	if err != nil {
		warnRing("listing workers", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var w string
		if rows.Scan(&w) == nil {
			out = append(out, w)
		}
	}
	return out
}
