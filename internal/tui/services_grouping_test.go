package tui

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/siteinfo"
)

func TestFailingWorkerNames_BuildsCanonicalKindSitePairs(t *testing.T) {
	snap := Snapshot{
		Sites: []siteinfo.EnrichedSite{
			{
				Name: "acme", HasQueueWorker: true, QueueFailing: true,
				HasScheduleWorker: true, ScheduleFailing: false,
				FrameworkWorkers: []siteinfo.WorkerInfo{
					{Name: "vite", Failing: true},
					{Name: "messenger", Failing: false},
				},
			},
			{
				Name: "shop", HasHorizon: true, HorizonFailing: true,
			},
		},
	}
	got := failingWorkerNames(snap)
	want := []string{"queue-acme", "vite-acme", "horizon-shop"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("failingWorkerNames = %v, want %v", got, want)
	}
}

func TestJoinTruncated(t *testing.T) {
	cases := []struct {
		in   []string
		max  int
		want string
	}{
		{[]string{"a", "b"}, 3, "a, b"},
		{[]string{"a", "b", "c"}, 3, "a, b, c"},
		{[]string{"a", "b", "c", "d", "e"}, 3, "a, b, c +2 more"},
		{[]string{}, 3, ""},
	}
	for _, c := range cases {
		if got := joinTruncated(c.in, c.max); got != c.want {
			t.Errorf("joinTruncated(%v, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
	}
}

func TestClassifyService(t *testing.T) {
	cases := []struct {
		row  ServiceRow
		want serviceGroup
	}{
		{ServiceRow{Name: "mysql"}, groupCore},
		{ServiceRow{Name: "phpmyadmin", Custom: true}, groupCustom},
		{ServiceRow{Name: "queue-acme", WorkerKind: "queue"}, groupWorkers},
		{ServiceRow{Name: "custom-only-pinned", Custom: true, Pinned: true}, groupCustom},
	}
	for _, c := range cases {
		if got := classifyService(c.row); got != c.want {
			t.Errorf("classifyService(%+v) = %v, want %v", c.row, got, c.want)
		}
	}
}

func TestRenderGroupedServiceRows_InsertsHeadersAndTracksCursor(t *testing.T) {
	services := []ServiceRow{
		{Name: "mysql", State: stateRunning},
		{Name: "redis", State: stateStopped},
		{Name: "phpmyadmin", State: stateRunning, Custom: true},
		{Name: "queue-acme", WorkerKind: "queue", State: stateRunning},
	}
	rows, cursorLine := renderGroupedServiceRows(services, 2, true, 80)
	joined := stripANSI(strings.Join(rows, "\n"))
	for _, header := range []string{"Core", "Custom", "Workers"} {
		if !strings.Contains(joined, header) {
			t.Errorf("expected %q header in output:\n%s", header, joined)
		}
	}
	// Cursor at index 2 (phpmyadmin) should map to a line index AFTER
	// the Custom header insertion (one blank + one header = +2 lines).
	if cursorLine < 4 {
		t.Errorf("expected cursorLine >= 4 (past Core + headers), got %d", cursorLine)
	}
	// The matching line should still contain the service name.
	if cursorLine >= len(rows) || !strings.Contains(stripANSI(rows[cursorLine]), "phpmyadmin") {
		t.Errorf("cursorLine %d should point at phpmyadmin, got %q", cursorLine, stripANSI(rows[cursorLine]))
	}
}

func TestRenderGroupedServiceRows_WorkersSubGroupBySite(t *testing.T) {
	// Pre-sorted the way filteredSortedServices delivers them: workers by site
	// then kind.
	services := []ServiceRow{
		{Name: "queue-acme", WorkerKind: "queue", WorkerSite: "acme", State: stateSuspended},
		{Name: "vite-acme", WorkerKind: "vite", WorkerSite: "acme", State: stateRunning},
		{Name: "queue-blog", WorkerKind: "queue", WorkerSite: "blog", State: stateRunning},
	}
	rows, _ := renderGroupedServiceRows(services, -1, false, 80)
	joined := stripANSI(strings.Join(rows, "\n"))

	// Each site appears once as a sub-header; the worker rows drop the -site
	// suffix and carry a state word.
	if strings.Count(joined, "acme") != 1 {
		t.Errorf("site acme should appear once as a sub-header, got:\n%s", joined)
	}
	if !strings.Contains(joined, "queue") || !strings.Contains(joined, "suspended") {
		t.Errorf("expected bare kind + state in worker rows:\n%s", joined)
	}
	if strings.Contains(joined, "queue-acme") {
		t.Errorf("worker rows should not repeat the site suffix:\n%s", joined)
	}
	// acme's sub-header must precede blog's (site-ordered).
	if strings.Index(joined, "acme") > strings.Index(joined, "blog") {
		t.Errorf("sites should be ordered, acme before blog:\n%s", joined)
	}
}

func TestSiteHasFailingWorker(t *testing.T) {
	cases := []struct {
		s    siteinfo.EnrichedSite
		want bool
	}{
		{siteinfo.EnrichedSite{Name: "ok"}, false},
		{siteinfo.EnrichedSite{Name: "queue-bad", QueueFailing: true}, true},
		{siteinfo.EnrichedSite{
			Name: "framework-bad",
			FrameworkWorkers: []siteinfo.WorkerInfo{
				{Name: "vite", Failing: true},
			},
		}, true},
		{siteinfo.EnrichedSite{
			Name: "worktree-bad",
			Worktrees: []siteinfo.WorktreeInfo{{
				FrameworkWorkers: []siteinfo.WorkerInfo{{Name: "vite", Failing: true}},
			}},
		}, true},
	}
	for _, c := range cases {
		if got := siteHasFailingWorker(c.s); got != c.want {
			t.Errorf("siteHasFailingWorker(%s) = %v, want %v", c.s.Name, got, c.want)
		}
	}
}
