package reqstats

import (
	"testing"
	"time"
)

func TestDeleteRouteRemovesHistoryAcrossWorktrees(t *testing.T) {
	s := tempStore(t)
	seed(t, s, mk(5, 0, "app", "GET", "GET /noisy", "/noisy", 200, 20))
	seed(t, s, mk(3, 0, "app/feature-x", "GET", "GET /noisy", "/noisy", 200, 20))
	seed(t, s, mk(4, 0, "app", "GET", "GET /keep", "/keep", 200, 20))
	seed(t, s, mk(2, 0, "other", "GET", "GET /noisy", "/noisy", 200, 20))

	n, err := s.DeleteRoute("app", "GET /noisy")
	if err != nil {
		t.Fatalf("DeleteRoute: %v", err)
	}
	if n != 8 {
		t.Errorf("deleted = %d, want 8 (site + worktree rows)", n)
	}
	a, _ := s.SiteAnalytics("app", base.Add(-time.Hour), base.Add(time.Hour))
	if a.Samples != 4 {
		t.Errorf("app has %d samples after delete, want 4", a.Samples)
	}
	o, _ := s.SiteAnalytics("other", base.Add(-time.Hour), base.Add(time.Hour))
	if o.Samples != 2 {
		t.Errorf("DeleteRoute reached another site: %d samples", o.Samples)
	}
}

func TestDeleteRequestRemovesOnlyThatRow(t *testing.T) {
	s := tempStore(t)
	seed(t, s, mk(3, 0, "app", "GET", "GET /page", "/page", 200, 20))

	target := base.Add(time.Second)
	n, err := s.DeleteRequest("app", target.UnixMilli(), "/page")
	if err != nil {
		t.Fatalf("DeleteRequest: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted = %d, want 1", n)
	}
	a, _ := s.SiteAnalytics("app", base.Add(-time.Hour), base.Add(time.Hour))
	if a.Samples != 2 {
		t.Errorf("samples = %d, want 2", a.Samples)
	}
}

func TestExcludedRoutesHiddenFromAnalyticsAndRecent(t *testing.T) {
	s := tempStore(t)
	seed(t, s, mk(5, 0, "app", "GET", "GET /health", "/health", 200, 20))
	seed(t, s, mk(4, 0, "app", "GET", "GET /keep", "/keep", 200, 20))

	if err := s.ExcludeRoute("app", "GET /health"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	a, err := s.SiteAnalytics("app", base.Add(-time.Hour), base.Add(time.Hour))
	if err != nil {
		t.Fatalf("SiteAnalytics: %v", err)
	}
	if a.Samples != 4 {
		t.Errorf("samples = %d, want 4 (excluded route hidden)", a.Samples)
	}
	for _, r := range a.Routes {
		if r.Route == "GET /health" {
			t.Error("excluded route still listed in analytics")
		}
	}
	recent, err := s.Recent("app", 20)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	for _, r := range recent {
		if r.Route == "GET /health" {
			t.Error("excluded route still listed in recent requests")
		}
	}
}

func TestExcludedRoutesApplyToWorktreeKeys(t *testing.T) {
	s := tempStore(t)
	seed(t, s, mk(5, 0, "app/feature-x", "GET", "GET /health", "/health", 200, 20))
	if err := s.ExcludeRoute("app", "GET /health"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	a, _ := s.SiteAnalytics("app/feature-x", base.Add(-time.Hour), base.Add(time.Hour))
	if a.Samples != 0 {
		t.Errorf("worktree still records %d samples for an excluded route", a.Samples)
	}
}

func TestExcludeRouteIsIdempotentAndReversible(t *testing.T) {
	s := tempStore(t)
	if err := s.ExcludeRoute("app", "GET /health"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	if err := s.ExcludeRoute("app", "GET /health"); err != nil {
		t.Fatalf("ExcludeRoute twice: %v", err)
	}
	got, err := s.ExcludedRoutes("app")
	if err != nil {
		t.Fatalf("ExcludedRoutes: %v", err)
	}
	if len(got) != 1 || got[0] != "GET /health" {
		t.Fatalf("excluded = %v, want one GET /health", got)
	}
	if err := s.UnexcludeRoute("app", "GET /health"); err != nil {
		t.Fatalf("UnexcludeRoute: %v", err)
	}
	got, _ = s.ExcludedRoutes("app")
	if len(got) != 0 {
		t.Errorf("excluded = %v after unexclude, want none", got)
	}
}

func TestExcludedRoutesAskedByWorktreeKey(t *testing.T) {
	s := tempStore(t)
	if err := s.ExcludeRoute("app/feature-x", "GET /health"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	got, _ := s.ExcludedRoutes("app")
	if len(got) != 1 {
		t.Errorf("a worktree exclusion must land on the site: %v", got)
	}
}

func TestAllExcludedRoutes(t *testing.T) {
	s := tempStore(t)
	if err := s.ExcludeRoute("app", "GET /health"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	if err := s.ExcludeRoute("app", "POST /ping"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	if err := s.ExcludeRoute("other", "GET /health"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	all, err := s.AllExcludedRoutes()
	if err != nil {
		t.Fatalf("AllExcludedRoutes: %v", err)
	}
	if len(all["app"]) != 2 || !all["app"]["GET /health"] || !all["app"]["POST /ping"] {
		t.Errorf("app excludes = %v", all["app"])
	}
	if len(all["other"]) != 1 {
		t.Errorf("other excludes = %v", all["other"])
	}
}

func TestDeleteSiteClearsItsExclusions(t *testing.T) {
	s := tempStore(t)
	if err := s.ExcludeRoute("gone", "GET /health"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	if err := s.ExcludeRoute("keep", "GET /health"); err != nil {
		t.Fatalf("ExcludeRoute: %v", err)
	}
	if _, err := s.DeleteSite("gone"); err != nil {
		t.Fatalf("DeleteSite: %v", err)
	}
	all, _ := s.AllExcludedRoutes()
	if _, ok := all["gone"]; ok {
		t.Error("DeleteSite left the site's exclusions behind")
	}
	if _, ok := all["keep"]; !ok {
		t.Error("DeleteSite dropped an unrelated site's exclusions")
	}
}

func TestAggregatorDropsExcludedRoutes(t *testing.T) {
	a := New(siteResolver(map[string]string{"app.test": "app"}))
	recordN(a, "app.test", "GET", "/health", 2000, 6)
	recordN(a, "app.test", "GET", "/keep", 2000, 6)

	a.SetExcluded(map[string]map[string]bool{"app": {"GET /health": true}})

	snap, ok := a.SiteSnapshot("app")
	if !ok {
		t.Fatal("site missing from the snapshot")
	}
	for _, r := range snap.Slow {
		if r.Route == "GET /health" {
			t.Error("an excluded route must not reach the slow list")
		}
	}
	// A record arriving after the exclusion is never taken in at all.
	recordN(a, "app.test", "GET", "/health", 3000, 6)
	snap, _ = a.SiteSnapshot("app")
	for _, r := range snap.Slow {
		if r.Route == "GET /health" {
			t.Error("an excluded route must not be recorded")
		}
	}
}

func TestAggregatorExcludedWorktreeKeys(t *testing.T) {
	a := New(siteResolver(map[string]string{"wt.test": "app/feature-x"}))
	recordN(a, "wt.test", "GET", "/health", 2000, 6)
	a.SetExcluded(map[string]map[string]bool{"app": {"GET /health": true}})
	snap, _ := a.SiteSnapshot("app/feature-x")
	for _, r := range snap.Slow {
		if r.Route == "GET /health" {
			t.Error("a site exclusion must cover its worktree keys")
		}
	}
}
