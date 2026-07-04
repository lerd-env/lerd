package sitedoctor

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/reqstats"
)

// seedSlowSnapshot registers a site at dir and writes a request-timing snapshot
// with one flagged route, returning the site path.
func seedSlowSnapshot(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	if err := config.AddSite(config.Site{Name: "acme", Path: dir, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	a := reqstats.New(func(h string) (string, bool) { return "acme", h == "acme.test" })
	for i := 0; i < 20; i++ {
		a.Record(reqstats.AccessRecord{Host: "acme.test", Status: 200, RequestTime: 0.04, Method: "GET", URI: "/home"})
	}
	for i := 0; i < 10; i++ {
		a.Record(reqstats.AccessRecord{Host: "acme.test", Status: 200, RequestTime: 0.5, Method: "GET", URI: "/reports/7"})
	}
	if err := a.Save(config.RequestStatsFile()); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCheckSlowRoutes_warnsWithFlaggedRoute(t *testing.T) {
	dir := seedSlowSnapshot(t)
	c, ok := checkSlowRoutes(dir)
	if !ok {
		t.Fatal("expected a slow-routes finding")
	}
	if c.Status != StatusWarn {
		t.Errorf("status = %q, want warn", c.Status)
	}
	if !strings.Contains(c.Detail, "GET /reports/:id") {
		t.Errorf("detail should name the slow route, got %q", c.Detail)
	}
	if c.Fix != "" {
		t.Errorf("slow routes have no command fix, got %q", c.Fix)
	}
}

func TestCheckSlowRoutes_silentWhenNoSnapshot(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	if err := config.AddSite(config.Site{Name: "acme", Path: dir, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := checkSlowRoutes(dir); ok {
		t.Error("no snapshot must yield no finding")
	}
}

func TestCheckSlowRoutes_silentForUnregisteredPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if _, ok := checkSlowRoutes(t.TempDir()); ok {
		t.Error("an unregistered (e.g. worktree) path must yield no finding")
	}
}
