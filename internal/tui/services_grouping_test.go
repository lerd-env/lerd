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
