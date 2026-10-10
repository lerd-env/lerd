package ui

import (
	"net/http"
	"strconv"

	"github.com/geodro/lerd/internal/dumps"
)

// The Debug lenses' endpoints. A lens asks for one page of groups and the
// counts for its badges; grouping, search and N+1 detection run in lerd-ui's
// SQLite buffer, so a tab holds only what it shows.

// lensGroupsPerPage and lensRowsPerGroup size a lens page: enough requests to
// fill the view, and the rows a group shows before "load more".
const (
	lensGroupsPerPage = 30
	lensRowsPerGroup  = 100
)

// lensScope reads the scope every lens endpoint shares: site, branch, kind,
// request and whether test runs show.
func lensScope(r *http.Request) dumps.FilterOpts {
	q := r.URL.Query()
	return dumps.FilterOpts{
		Site:      resolveSiteName(q.Get("site")),
		Branch:    q.Get("branch"),
		Ctx:       q.Get("ctx"),
		Kind:      q.Get("kind"),
		RID:       q.Get("rid"),
		Route:     q.Get("route"),
		HideTests: q.Get("tests") != "1",
	}
}

func lensOpts(r *http.Request) dumps.GroupOpts {
	q := r.URL.Query()
	opts := dumps.GroupOpts{
		FilterOpts:  lensScope(r),
		Search:      q.Get("q"),
		Worker:      q.Get("worker"),
		HideWorkers: q.Get("workers") == "0",
		Facet:       q.Get("facet"),
		Limit:       lensGroupsPerPage,
		Rows:        lensRowsPerGroup,
	}
	opts.Before, _ = strconv.ParseInt(q.Get("before"), 10, 64)
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n < lensGroupsPerPage {
		opts.Limit = n
	}
	return opts
}

// lensServer refuses anything but a GET, reporting false once it has answered.
func lensServer(w http.ResponseWriter, r *http.Request) (*dumps.Server, bool) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return nil, false
	}
	return dumpsServer.Load(), true
}

// handleDumpsCounts answers the lens bar's badges for a site.
func handleDumpsCounts(w http.ResponseWriter, r *http.Request) {
	srv, ok := lensServer(w, r)
	if !ok {
		return
	}
	resp := struct {
		Counts      map[string]int `json:"counts"`
		HiddenTests int            `json:"hidden_tests"`
	}{Counts: map[string]int{}}
	if srv != nil {
		scope := lensScope(r)
		resp.Counts = srv.Counts(scope)
		if scope.HideTests {
			resp.HiddenTests = srv.TestCount(scope)
		}
	}
	writeJSON(w, resp)
}

// handleDumpsGroups answers one page of a lens.
func handleDumpsGroups(w http.ResponseWriter, r *http.Request) {
	srv, ok := lensServer(w, r)
	if !ok {
		return
	}
	if srv == nil {
		writeJSON(w, dumps.GroupPage{Groups: []dumps.Group{}})
		return
	}
	writeJSON(w, srv.Groups(lensOpts(r)))
}

// handleDumpsGroupRows answers more rows of one group, from offset on.
func handleDumpsGroupRows(w http.ResponseWriter, r *http.Request) {
	srv, ok := lensServer(w, r)
	if !ok {
		return
	}
	if srv == nil {
		writeJSON(w, []dumps.Row{})
		return
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	writeJSON(w, srv.GroupRows(lensOpts(r), r.URL.Query().Get("key"), offset))
}

// handleDumpsFacets answers what a lens can filter by: the sites, the worker
// commands, and with a kind, its facet values (job statuses, log levels…).
func handleDumpsFacets(w http.ResponseWriter, r *http.Request) {
	srv, ok := lensServer(w, r)
	if !ok {
		return
	}
	resp := struct {
		Sites   []string `json:"sites"`
		Workers []string `json:"workers"`
		Values  []string `json:"values"`
	}{Sites: []string{}, Workers: []string{}, Values: []string{}}
	if srv != nil {
		scope := lensScope(r)
		resp.Sites = srv.Sites()
		kind := scope.Kind
		scope.Kind = ""
		resp.Workers = srv.Workers(scope)
		if kind != "" {
			scope.Kind = kind
			resp.Values = srv.FacetValues(scope)
		}
	}
	writeJSON(w, resp)
}

// handleDumpsEvent answers one whole event, for a row a lens opens: the list
// leaves out its call stack and a mail's HTML.
func handleDumpsEvent(w http.ResponseWriter, r *http.Request) {
	srv, ok := lensServer(w, r)
	if !ok {
		return
	}
	if srv == nil {
		http.NotFound(w, r)
		return
	}
	data, found := srv.Event(r.URL.Query().Get("id"))
	if !found {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}
