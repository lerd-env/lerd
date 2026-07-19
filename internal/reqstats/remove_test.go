package reqstats

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRemoveSiteSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "request-stats.json")
	snap := []SiteStats{
		{Site: "keep"},
		{Site: "gone"},
		{Site: "gone/feature-x"}, // worktree key of the removed site
		{Site: "gone-ish"},       // shares a prefix but is a different site
	}
	if err := SaveSnapshot(snap, path); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if err := RemoveSite(path, "gone"); err != nil {
		t.Fatalf("RemoveSite: %v", err)
	}
	got := Load(path)
	names := map[string]bool{}
	for _, s := range got {
		names[s.Site] = true
	}
	if names["gone"] || names["gone/feature-x"] {
		t.Errorf("RemoveSite left the site or its worktree behind: %+v", got)
	}
	if !names["keep"] || !names["gone-ish"] {
		t.Errorf("RemoveSite dropped an unrelated site: %+v", got)
	}
}

func TestRemoveSiteMissingFile(t *testing.T) {
	if err := RemoveSite(filepath.Join(t.TempDir(), "nope.json"), "x"); err != nil {
		t.Errorf("RemoveSite on a missing file must be a no-op, got %v", err)
	}
}

func TestStoreDeleteSite(t *testing.T) {
	s := tempStore(t)
	seed(t, s, mk(5, 0, "gone", "GET", "GET /a", "/a", 200, 20))
	seed(t, s, mk(3, 0, "gone/feature-x", "GET", "GET /b", "/b", 200, 20))
	seed(t, s, mk(4, 0, "keep", "GET", "GET /c", "/c", 200, 20))

	n, err := s.DeleteSite("gone")
	if err != nil {
		t.Fatalf("DeleteSite: %v", err)
	}
	if n != 8 {
		t.Errorf("deleted = %d, want 8 (site + worktree rows)", n)
	}
	a, err := s.SiteAnalytics("gone", base.Add(-time.Hour), base.Add(time.Hour))
	if err != nil {
		t.Fatalf("SiteAnalytics: %v", err)
	}
	if a.Samples != 0 {
		t.Errorf("gone still has %d samples after delete", a.Samples)
	}
	keep, _ := s.SiteAnalytics("keep", base.Add(-time.Hour), base.Add(time.Hour))
	if keep.Samples != 4 {
		t.Errorf("keep lost rows: %d samples, want 4", keep.Samples)
	}
}

func TestAggregatorForget(t *testing.T) {
	a := New(siteResolver(map[string]string{
		"gone.test": "gone",
		"wt.test":   "gone/feature-x", // a worktree of the removed site
		"keep.test": "keep",
	}))
	recordN(a, "gone.test", "GET", "/home", 40, 10)
	recordN(a, "wt.test", "GET", "/home", 40, 10)
	recordN(a, "keep.test", "GET", "/home", 40, 10)

	a.Forget("gone")
	if _, ok := a.SiteSnapshot("gone"); ok {
		t.Error("Forget must drop the site from the aggregator")
	}
	if _, ok := a.SiteSnapshot("gone/feature-x"); ok {
		t.Error("Forget must drop the site's worktree keys too")
	}
	if _, ok := a.SiteSnapshot("keep"); !ok {
		t.Error("Forget must not touch an unrelated site")
	}
}
