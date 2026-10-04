package ui

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/browsercapture"
	"github.com/geodro/lerd/internal/dumps"
)

// RequestLink names a page view that sent a request, or a request a page sent:
// its request id, the site it ran on and how it was sent.
type RequestLink struct {
	RID    string `json:"rid"`
	Site   string `json:"site,omitempty"`
	URL    string `json:"url,omitempty"`
	Via    string `json:"via,omitempty"`
	Status int    `json:"status,omitempty"`
	Cross  bool   `json:"cross_origin,omitempty"`
	// At and DurationMS are when the browser got the response and how long
	// the call took, and Timing its phases as the browser saw them.
	At         string             `json:"at,omitempty"`
	DurationMS float64            `json:"duration_ms,omitempty"`
	Timing     map[string]float64 `json:"timing,omitempty"`
}

// RequestSummary is one request as a list row: what it was, how it ended and
// what happened in it, plus the page view that sent it when a browser did.
type RequestSummary struct {
	RID    string `json:"rid"`
	Type   string `json:"type"`
	Site   string `json:"site,omitempty"`
	Branch string `json:"branch,omitempty"`
	Method string `json:"method,omitempty"`
	URI    string `json:"uri,omitempty"`
	Route  string `json:"route,omitempty"`
	// NginxMS is how long nginx held the request before handing it to PHP and
	// QueueMS how long it then waited for a free FPM worker.
	NginxMS float64 `json:"nginx_ms,omitempty"`
	QueueMS float64 `json:"queue_ms,omitempty"`
	// Job and JobStatus name a queued job run as its own process, and how it
	// ended: processed, failed, errored with attempts left, or still running.
	Job       string         `json:"job,omitempty"`
	JobStatus string         `json:"job_status,omitempty"`
	Status    int            `json:"status,omitempty"`
	TimeMS    float64        `json:"time_ms,omitempty"`
	Started   string         `json:"started"`
	Worker    string         `json:"worker,omitempty"`
	Command   string         `json:"command,omitempty"`
	Counts    map[string]int `json:"counts"`
	Problems  []string       `json:"problems"`
	Parent    *RequestLink   `json:"parent,omitempty"`
	Children  []RequestLink  `json:"children,omitempty"`
}

// RequestDetail is a request with everything that carried its id, by kind.
type RequestDetail struct {
	RequestSummary
	Events  map[string][]dumps.Event `json:"events"`
	Queries *RequestAnalysis         `json:"queries,omitempty"`
}

// jobData is what a job event says about the job it reports.
type jobData struct {
	Status     string  `json:"status"`
	Class      string  `json:"class"`
	Connection string  `json:"connection"`
	Parent     string  `json:"parent"`
	TimeMS     float64 `json:"time_ms"`
}

type requestAcc struct {
	rid     string
	events  []dumps.Event
	browser []browsercapture.Report
}

// collectRequests groups events by request id and links page views to the
// requests they sent, across sites, through the ids a browser read off its
// responses.
func collectRequests(events []dumps.Event) (map[string]*requestAcc, []string, map[string]*RequestLink, map[string][]RequestLink) {
	accs := map[string]*requestAcc{}
	var order []string
	parents := map[string]*RequestLink{}
	children := map[string][]RequestLink{}
	for _, e := range events {
		rid := e.Ctx.RID
		if rid == "" {
			continue
		}
		a := accs[rid]
		if a == nil {
			a = &requestAcc{rid: rid}
			accs[rid] = a
			order = append(order, rid)
		}
		a.events = append(a.events, e)
		// A sync job names the request it ran in, which lists it as a child.
		if e.Kind == dumps.KindJob {
			var j jobData
			if json.Unmarshal(e.Data, &j) == nil && j.Status == "processing" && j.Parent != "" && j.Parent != rid {
				children[j.Parent] = append(children[j.Parent], RequestLink{RID: rid, URL: j.Class, Via: "job", At: e.TS})
				parents[rid] = &RequestLink{RID: j.Parent, Site: e.Ctx.Site, Via: "job"}
			}
			continue
		}
		if e.Kind != dumps.KindBrowser {
			continue
		}
		var r browsercapture.Report
		if json.Unmarshal(e.Data, &r) != nil {
			continue
		}
		a.browser = append(a.browser, r)
		if r.Type == "request" && r.RID != "" && r.RID != rid {
			child := RequestLink{RID: r.RID, URL: r.Request, Via: r.Via, Status: r.Status, Cross: r.Cross, At: e.TS, DurationMS: r.Duration}
			children[rid] = append(children[rid], child)
			parents[r.RID] = &RequestLink{RID: rid, Site: e.Ctx.Site, URL: e.Ctx.Request, Via: r.Via, Cross: r.Cross, At: e.TS, DurationMS: r.Duration, Timing: r.Timing}
		}
	}
	return accs, order, parents, children
}

func summarize(a *requestAcc, parent *RequestLink, kids []RequestLink) RequestSummary {
	s := RequestSummary{RID: a.rid, Counts: map[string]int{}, Problems: []string{}, Parent: parent, Children: kids}
	problems := map[string]bool{}
	startOf := ""
	var queries []dumps.Event
	page := false
	for _, e := range a.events {
		if s.Started == "" || e.TS < s.Started {
			s.Started = e.TS
		}
		if s.Site == "" {
			s.Site, s.Branch = e.Ctx.Site, e.Ctx.Branch
		}
		switch e.Ctx.Type {
		case "cli":
			s.Type, s.Worker, s.Command = "cli", e.Ctx.Worker, e.Ctx.Command
		case "fpm":
			if s.Method == "" && e.Ctx.Request != "" {
				s.Method, s.URI, _ = strings.Cut(e.Ctx.Request, " ")
			}
		}
		switch e.Kind {
		case dumps.KindRequest:
			var d struct {
				Method string  `json:"method"`
				URI    string  `json:"uri"`
				Status int     `json:"status"`
				TimeMS float64 `json:"time_ms"`
				Route  string  `json:"route"`
				Nginx  float64 `json:"nginx_ms"`
				Queue  float64 `json:"queue_ms"`
			}
			if json.Unmarshal(e.Data, &d) == nil {
				s.Method, s.URI, s.Status, s.TimeMS, s.Route = d.Method, d.URI, d.Status, d.TimeMS, d.Route
				s.NginxMS, s.QueueMS = d.Nginx, d.Queue
				// The event is sent when the request ends, so its start is that
				// moment less the time it took.
				if end, err := time.Parse(time.RFC3339Nano, e.TS); err == nil {
					startOf = end.Add(-time.Duration(d.TimeMS * float64(time.Millisecond))).UTC().Format("2006-01-02T15:04:05.000Z")
				}
			}
			continue
		case dumps.KindQuery:
			queries = append(queries, e)
		case dumps.KindJob:
			var j jobData
			if json.Unmarshal(e.Data, &j) == nil {
				if j.Status == "processing" {
					s.Job = j.Class
				}
				if s.Job != "" && j.Class == s.Job {
					s.JobStatus = j.Status
					if j.TimeMS > 0 {
						s.TimeMS = j.TimeMS
					}
				}
			}
		case dumps.KindException:
			problems["exception"] = true
		case dumps.KindLog:
			var d struct {
				Level string `json:"level"`
			}
			if json.Unmarshal(e.Data, &d) == nil && logIsError(d.Level) {
				problems["log error"] = true
			}
		}
		s.Counts[e.Kind]++
	}
	for _, r := range a.browser {
		switch browsercapture.EventType(r) {
		case "navigation", "timing":
			page = true
			if s.URI == "" {
				s.URI = r.URL
			}
		case "error", "rejection", "console.error":
			problems["js error"] = true
		case "network", "resource":
			problems["failed request"] = true
		}
	}
	if s.JobStatus == "failed" || s.JobStatus == "errored" {
		problems["job "+s.JobStatus] = true
	}
	switch {
	case s.Job != "":
		s.Type = "job"
	case s.Type == "cli" && s.Worker != "":
		s.Type = "worker"
	case s.Type == "cli":
	case page:
		s.Type = "page"
	case parent != nil && parent.Via != "":
		s.Type = parent.Via
	default:
		s.Type = "request"
	}
	if s.Status >= 500 {
		problems["5xx"] = true
	} else if s.Status >= 400 {
		problems["4xx"] = true
	}
	if len(queries) > 0 {
		for _, ra := range analyzeQueries(queries, 0, 0).Requests {
			if len(ra.NPlusOne) > 0 {
				problems["N+1"] = true
			}
			if len(ra.Slow) > 0 {
				problems["slow query"] = true
			}
		}
	}
	if startOf != "" {
		s.Started = startOf
	}
	for p := range problems {
		s.Problems = append(s.Problems, p)
	}
	sort.Strings(s.Problems)
	return s
}

func logIsError(level string) bool {
	switch level {
	case "error", "critical", "alert", "emergency":
		return true
	}
	return false
}

// listRequests returns the requests in events, newest first.
func listRequests(events []dumps.Event) []RequestSummary {
	accs, order, parents, children := collectRequests(events)
	out := make([]RequestSummary, 0, len(order))
	for _, rid := range order {
		out = append(out, summarize(accs[rid], parents[rid], children[rid]))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Started > out[j].Started })
	return out
}

// requestDetail returns one request with everything that carried its id.
func requestDetail(events []dumps.Event, rid string) (RequestDetail, bool) {
	accs, _, parents, children := collectRequests(events)
	a := accs[rid]
	if a == nil {
		return RequestDetail{}, false
	}
	d := RequestDetail{RequestSummary: summarize(a, parents[rid], children[rid]), Events: map[string][]dumps.Event{}}
	var queries []dumps.Event
	for _, e := range a.events {
		d.Events[e.Kind] = append(d.Events[e.Kind], e)
		if e.Kind == dumps.KindQuery {
			queries = append(queries, e)
		}
	}
	if len(queries) > 0 {
		ra := analyzeQueriesAll(queries)
		d.Queries = &ra
	}
	return d, true
}

// analyzeQueriesAll is the query analysis of one request, kept even when it
// found nothing, so the detail view always has the totals.
func analyzeQueriesAll(queries []dumps.Event) RequestAnalysis {
	for _, ra := range analyzeQueries(queries, 0, 0).Requests {
		return ra
	}
	ra := RequestAnalysis{RID: queries[0].Ctx.RID, QueryCount: len(queries)}
	for _, e := range queries {
		if q, ok := e.Query(); ok {
			ra.TotalTimeMS += q.TimeMS
		}
	}
	return ra
}

// handleRequests lists requests (?site=, ?branch=, ?limit=) at /api/requests,
// or returns one at /api/requests/{rid}.
func handleRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	srv := dumpsServer.Load()
	var events []dumps.Event
	if srv != nil {
		events = srv.Snapshot()
	}
	if rid := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/requests"), "/"); rid != "" {
		d, ok := requestDetail(events, rid)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, d)
		return
	}
	q := r.URL.Query()
	site, branch := resolveSiteName(q.Get("site")), q.Get("branch")
	limit, _ := strconv.Atoi(q.Get("limit"))
	out := []RequestSummary{}
	for _, s := range listRequests(events) {
		if (site != "" && s.Site != site) || (branch != "" && s.Branch != branch) {
			continue
		}
		out = append(out, s)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	writeJSON(w, out)
}
