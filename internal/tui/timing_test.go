package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/reqstats"
	"github.com/geodro/lerd/internal/siteinfo"
)

func timingSite() *siteinfo.EnrichedSite {
	return &siteinfo.EnrichedSite{
		Name:   "alpha",
		Branch: "main",
		Worktrees: []siteinfo.WorktreeInfo{
			{Branch: "feature-x"},
			{Branch: "hotfix"},
		},
	}
}

func TestTimingScopes_KeyWorktreesSeparatelyFromTheSite(t *testing.T) {
	scopes := timingScopes(timingSite())
	if len(scopes) != 3 {
		t.Fatalf("expected the site plus its two worktrees, got %d scopes", len(scopes))
	}
	// The site reads on its bare name; a worktree reads on site/branch, the same
	// identity the watcher records under.
	if scopes[0].key != "alpha" {
		t.Errorf("main scope should key on the site name, got %q", scopes[0].key)
	}
	if scopes[1].key != "alpha/feature-x" {
		t.Errorf("worktree scope should key on site/branch, got %q", scopes[1].key)
	}
	if scopes[1].label != "feature-x" {
		t.Errorf("worktree scope should be labelled by branch, got %q", scopes[1].label)
	}
}

func TestTimingScopes_SiteWithoutWorktrees(t *testing.T) {
	scopes := timingScopes(&siteinfo.EnrichedSite{Name: "solo"})
	if len(scopes) != 1 {
		t.Fatalf("a site with no worktrees has one scope, got %d", len(scopes))
	}
	if scopes[0].label != "main" {
		t.Errorf("a site with no detected branch should fall back to main, got %q", scopes[0].label)
	}
}

func TestCycleTimingScope_WrapsAndIsNoOpWithoutWorktrees(t *testing.T) {
	m := NewModel("test")
	m.activeTab = tabSites
	m.snap = Snapshot{Sites: []siteinfo.EnrichedSite{*timingSite()}}

	m.cycleTimingScope(1)
	if m.timingScope != 1 {
		t.Fatalf("b should advance to the first worktree, got %d", m.timingScope)
	}
	m.cycleTimingScope(1)
	m.cycleTimingScope(1)
	if m.timingScope != 0 {
		t.Fatalf("the branch cycle should wrap back to the site, got %d", m.timingScope)
	}

	m.snap = Snapshot{Sites: []siteinfo.EnrichedSite{{Name: "solo"}}}
	m.cycleTimingScope(1)
	if m.timingScope != 0 {
		t.Fatalf("a site with no worktrees has nothing to cycle, got %d", m.timingScope)
	}
}

func TestCurrentTimingScope_ClampsWhenMovingToASiteWithFewerWorktrees(t *testing.T) {
	m := NewModel("test")
	m.activeTab = tabSites
	m.snap = Snapshot{Sites: []siteinfo.EnrichedSite{*timingSite()}}
	m.timingScope = 2 // parked on the second worktree

	// Navigating to a site with no worktrees must not index out of range.
	m.snap = Snapshot{Sites: []siteinfo.EnrichedSite{{Name: "solo"}}}
	scope, ok := m.currentTimingScope()
	if !ok {
		t.Fatal("expected a scope for the focused site")
	}
	if scope.key != "solo" {
		t.Fatalf("scope should clamp back to the site, got %q", scope.key)
	}
}

func TestTimingCacheKey_ChangesWithBranchAndWindow(t *testing.T) {
	m := NewModel("test")
	m.activeTab = tabSites
	m.snap = Snapshot{Sites: []siteinfo.EnrichedSite{*timingSite()}}

	base := m.timingCacheKey()
	m.cycleTimingRange(1)
	if widened := m.timingCacheKey(); widened == base {
		t.Fatal("changing the window must invalidate the cached figures")
	}
	m.cycleTimingRange(-1)
	m.cycleTimingScope(1)
	if branched := m.timingCacheKey(); branched == base {
		t.Fatal("changing the branch must invalidate the cached figures")
	}
}

func TestTimingResult_LateReadForAnotherScopeIsDiscarded(t *testing.T) {
	m := NewModel("test")
	m.activeTab = tabSites
	m.snap = Snapshot{Sites: []siteinfo.EnrichedSite{*timingSite()}}
	m.timingKey = "alpha@1h"

	// A read that finishes after the user has cycled to a worktree must not land
	// under the new heading.
	m.Update(timingResultMsg{cacheKey: "alpha/feature-x@1h", analytic: reqstats.Analytics{Samples: 99}})
	if m.timingLoaded {
		t.Fatal("a result for another scope should be discarded")
	}
	m.Update(timingResultMsg{cacheKey: "alpha@1h", analytic: reqstats.Analytics{Samples: 42}})
	if !m.timingLoaded || m.timing.Samples != 42 {
		t.Fatalf("the result for the focused scope should land, got loaded=%v samples=%d",
			m.timingLoaded, m.timing.Samples)
	}
}

func TestTimingSection_EmptyAndPopulatedStates(t *testing.T) {
	m := NewModel("test")
	m.activeTab = tabSites
	site := timingSite()
	m.snap = Snapshot{Sites: []siteinfo.EnrichedSite{*site}}

	// Before the first read lands the panel reads as loading, not as "no traffic".
	got := stripANSI(strings.Join(timingSectionLines(m, site, 90), "\n"))
	if !strings.Contains(got, "reading…") {
		t.Fatalf("expected a loading state before the first read:\n%s", got)
	}

	m.timingLoaded = true
	got = stripANSI(strings.Join(timingSectionLines(m, site, 90), "\n"))
	if !strings.Contains(got, "no requests in the last 1h") {
		t.Fatalf("expected an empty state naming the window:\n%s", got)
	}

	m.timing = reqstats.Analytics{
		Samples:      12,
		ColdStarts:   1,
		MedianMillis: 18.4,
		P95Millis:    240,
		Status:       reqstats.StatusCounts{C2xx: 10, C5xx: 2},
		Distribution: []reqstats.LatencyBucket{{UpperMillis: 25, Count: 8}, {UpperMillis: 0, Count: 4}},
		// Route carries the method, the way reqstats.NormalizeRoute emits it.
		Routes: []reqstats.RouteStat{
			{Route: "GET /fast", Method: "GET", RecentP95Millis: 12, Samples: 9},
			{Route: "POST /slow", Method: "POST", RecentP95Millis: 980, Samples: 3},
		},
	}
	m.timingRecent = []reqstats.Record{
		{At: time.Now(), Method: "GET", URI: "/checkout", Status: 500, Millis: 1400},
	}
	got = stripANSI(strings.Join(timingSectionLines(m, site, 90), "\n"))

	for _, want := range []string{
		"p50 18.4ms", "p95 240ms", "12 requests", "1 cold", // headline
		"2xx 10", "5xx 2", // status mix
		"Response times", "<25ms", "≥1s", // distribution
		"Slowest routes", "POST /slow", "980ms", // routes, ranked by recent p95
		"Recent", "/checkout", "1.40s", // recent tail, seconds for slow requests
	} {
		if !strings.Contains(got, want) {
			t.Errorf("timing panel missing %q:\n%s", want, got)
		}
	}
	// The slowest route must outrank the fast one.
	if strings.Index(got, "POST /slow") > strings.Index(got, "GET /fast") {
		t.Errorf("routes should rank slowest first:\n%s", got)
	}
	// The method lives in Route; it must not also be printed as its own column.
	if strings.Contains(got, "GET       GET ") {
		t.Errorf("route rows should not repeat the method:\n%s", got)
	}
}

func TestTimingPanel_ScopedToTheOverviewTab(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap()
	m.activeTab = tabSites
	m.siteTab = tabSiteOverview
	if !m.timingActive() {
		t.Fatal("the timing panel belongs on the site Overview")
	}
	m.siteTab = tabSiteLogs
	if m.timingActive() {
		t.Fatal("the timing panel should not load on the Logs tab")
	}
	m.siteTab = tabSiteOverview
	m.activeTab = tabServices
	if m.timingActive() {
		t.Fatal("the timing panel should not load on the Services tab")
	}
}
