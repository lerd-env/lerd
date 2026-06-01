package ui

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/geodro/lerd/internal/dumps"
)

// defaultSlowQueryMS mirrors the dashboard's slow-query tag threshold.
const defaultSlowQueryMS = 100.0

// AnalyzedCaller is the application frame a query originated from — the line a
// fixer opens to add eager loading, an index, or a cache.
type AnalyzedCaller struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// NPlusOneFinding is one query shape that repeated past the threshold inside a
// single request — the classic N+1. Fingerprint is the literal-collapsed SQL
// shared by every repeat; SampleSQL is one concrete instance.
type NPlusOneFinding struct {
	Fingerprint string         `json:"fingerprint"`
	Count       int            `json:"count"`
	TotalTimeMS float64        `json:"total_time_ms"`
	SampleSQL   string         `json:"sample_sql"`
	Caller      AnalyzedCaller `json:"caller"`
}

// SlowFinding is a single query at or over the slow threshold.
type SlowFinding struct {
	SQL    string         `json:"sql"`
	TimeMS float64        `json:"time_ms"`
	Caller AnalyzedCaller `json:"caller"`
}

// RequestAnalysis groups findings for one request (or worker invocation).
type RequestAnalysis struct {
	Site        string            `json:"site,omitempty"`
	Request     string            `json:"request,omitempty"`
	Worker      string            `json:"worker,omitempty"`
	RID         string            `json:"rid,omitempty"`
	QueryCount  int               `json:"query_count"`
	TotalTimeMS float64           `json:"total_time_ms"`
	NPlusOne    []NPlusOneFinding `json:"n_plus_one,omitempty"`
	Slow        []SlowFinding     `json:"slow,omitempty"`
}

// QueryAnalysis is the full report: only requests with at least one finding are
// listed, so an agent gets an actionable work-list rather than a dump.
type QueryAnalysis struct {
	Requests []RequestAnalysis `json:"requests"`
	Summary  struct {
		RequestsAnalyzed int `json:"requests_analyzed"`
		NPlusOneFindings int `json:"n_plus_one_findings"`
		SlowFindings     int `json:"slow_findings"`
	} `json:"summary"`
}

// reqAcc accumulates per-request analysis state while walking the event list.
type reqAcc struct {
	site, request, worker, rid string
	queryCount                 int
	totalMS                    float64
	order                      []string                  // fingerprint first-seen order
	count                      map[string]int            // fingerprint -> count
	timeMS                     map[string]float64        // fingerprint -> summed time
	sample                     map[string]string         // fingerprint -> a concrete SQL
	caller                     map[string]AnalyzedCaller // fingerprint -> originating frame
	slow                       []SlowFinding
}

// analyzeQueries turns captured query events into an N+1 / slow-query report.
// Pure (no I/O) so it's unit-testable. Groups by the per-request id when the
// extension provided one, falling back to the route/worker key for events that
// predate the request boundary. minRepeat is the N+1 threshold; slowMS flags
// individual slow queries.
func analyzeQueries(events []dumps.Event, minRepeat int, slowMS float64) QueryAnalysis {
	if minRepeat < 2 {
		minRepeat = nPlusOneThreshold
	}
	if slowMS <= 0 {
		slowMS = defaultSlowQueryMS
	}

	accs := map[string]*reqAcc{}
	var keyOrder []string
	for _, ev := range events {
		q, ok := ev.Query()
		if !ok || q.SQL == "" {
			continue
		}
		key := ev.Ctx.RID
		if key == "" {
			key = routeKeyForQuery(ev)
		}
		a := accs[key]
		if a == nil {
			a = &reqAcc{
				site: ev.Ctx.Site, request: ev.Ctx.Request, worker: ev.Ctx.Worker, rid: ev.Ctx.RID,
				count: map[string]int{}, timeMS: map[string]float64{},
				sample: map[string]string{}, caller: map[string]AnalyzedCaller{},
			}
			accs[key] = a
			keyOrder = append(keyOrder, key)
		}
		a.queryCount++
		a.totalMS += q.TimeMS
		fp := normalizeSQL(q.SQL)
		if _, seen := a.count[fp]; !seen {
			a.order = append(a.order, fp)
			a.sample[fp] = q.SQL
			a.caller[fp] = AnalyzedCaller{File: ev.Src.File, Line: ev.Src.Line}
		}
		a.count[fp]++
		a.timeMS[fp] += q.TimeMS
		if q.TimeMS >= slowMS {
			a.slow = append(a.slow, SlowFinding{
				SQL: q.SQL, TimeMS: q.TimeMS,
				Caller: AnalyzedCaller{File: ev.Src.File, Line: ev.Src.Line},
			})
		}
	}

	var report QueryAnalysis
	for _, key := range keyOrder {
		a := accs[key]
		var nplus []NPlusOneFinding
		for _, fp := range a.order {
			if a.count[fp] < minRepeat {
				continue
			}
			nplus = append(nplus, NPlusOneFinding{
				Fingerprint: fp,
				Count:       a.count[fp],
				TotalTimeMS: a.timeMS[fp],
				SampleSQL:   a.sample[fp],
				Caller:      a.caller[fp],
			})
		}
		if len(nplus) == 0 && len(a.slow) == 0 {
			continue
		}
		report.Requests = append(report.Requests, RequestAnalysis{
			Site: a.site, Request: a.request, Worker: a.worker, RID: a.rid,
			QueryCount: a.queryCount, TotalTimeMS: a.totalMS,
			NPlusOne: nplus, Slow: a.slow,
		})
		report.Summary.NPlusOneFindings += len(nplus)
		report.Summary.SlowFindings += len(a.slow)
	}
	report.Summary.RequestsAnalyzed = len(report.Requests)
	// Worst offenders first: most N+1 shapes, then slowest cumulative time.
	sort.SliceStable(report.Requests, func(i, j int) bool {
		ri, rj := report.Requests[i], report.Requests[j]
		if len(ri.NPlusOne) != len(rj.NPlusOne) {
			return len(ri.NPlusOne) > len(rj.NPlusOne)
		}
		return ri.TotalTimeMS > rj.TotalTimeMS
	})
	if report.Requests == nil {
		report.Requests = []RequestAnalysis{}
	}
	return report
}

// handleQueriesAnalyze serves the N+1 / slow-query report over the captured
// query ring. Read-only GET; available to the dashboard, CLI, and MCP. Query
// params: site (filter), min_repeat (N+1 threshold), slow_ms (slow cutoff).
func handleQueriesAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	srv := dumpsServer.Load()
	if srv == nil {
		writeJSON(w, QueryAnalysis{Requests: []RequestAnalysis{}})
		return
	}
	q := r.URL.Query()
	minRepeat, _ := strconv.Atoi(q.Get("min_repeat"))
	slowMS, _ := strconv.ParseFloat(q.Get("slow_ms"), 64)
	events := srv.Filter(dumps.FilterOpts{Site: q.Get("site"), Kind: dumps.KindQuery})
	writeJSON(w, analyzeQueries(events, minRepeat, slowMS))
}
