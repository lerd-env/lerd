package siteinfo

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestEnrichWorkers_excludesPerWorktreeFromParent pins the regression that
// surfaced after #319 wired per_worktree workers: enrichWorkers iterated
// every worker in fw.Workers and queried `lerd-<n>-<site>`, which for
// per_worktree workers (vite) is a unit that never exists on the parent.
// The dashboard rendered a perma-inactive vite row on the parent site.
// Parent-only workers must skip anything with per_worktree:true.
func TestEnrichWorkers_excludesPerWorktreeFromParent(t *testing.T) {
	origUnit := unitStatusFn
	unitStatusFn = func(string) (string, error) { return "inactive", nil }
	defer func() { unitStatusFn = origUnit }()

	tr := true
	fw := &config.Framework{
		Workers: map[string]config.FrameworkWorker{
			"vite":    {Label: "Vite", Command: "npm run dev", PerWorktree: &tr},
			"reverb":  {Label: "Reverb", Command: "php artisan reverb:start"},
			"horizon": {Label: "Horizon", Command: "php artisan horizon"},
		},
	}

	// Use the real theregistry.test fixture's site name so the failure
	// signal carries production context if a regression slips in.
	e := &EnrichedSite{Name: "whitewaters", Path: "/projects/whitewaters"}
	e.enrichWorkers(fw, true)

	for _, w := range e.FrameworkWorkers {
		if w.Name == "vite" {
			t.Errorf("vite (per_worktree:true) leaked into parent FrameworkWorkers: %+v", e.FrameworkWorkers)
		}
	}
}

// TestEnrichWorkers_keepsNonPerWorktreeOnParent makes sure the new
// per_worktree gate doesn't accidentally hide a custom non-per-worktree
// worker that the framework yaml ships (e.g. a "search-indexer" daemon).
func TestEnrichWorkers_keepsNonPerWorktreeOnParent(t *testing.T) {
	origUnit := unitStatusFn
	unitStatusFn = func(name string) (string, error) {
		if name == "lerd-search-indexer-whitewaters" {
			return "active", nil
		}
		return "inactive", nil
	}
	defer func() { unitStatusFn = origUnit }()

	fw := &config.Framework{
		Workers: map[string]config.FrameworkWorker{
			"search-indexer": {Label: "Search indexer", Command: "php artisan scout:work"},
		},
	}

	e := &EnrichedSite{Name: "whitewaters", Path: "/projects/whitewaters"}
	e.enrichWorkers(fw, true)

	if len(e.FrameworkWorkers) != 1 || e.FrameworkWorkers[0].Name != "search-indexer" {
		t.Fatalf("expected search-indexer on parent, got %+v", e.FrameworkWorkers)
	}
	if !e.FrameworkWorkers[0].Running {
		t.Errorf("parent worker status not propagated, want running=true: %+v", e.FrameworkWorkers[0])
	}
}
