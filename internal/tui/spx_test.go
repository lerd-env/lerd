package tui

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/reqstats"
	"github.com/geodro/lerd/internal/siteinfo"
	"github.com/geodro/lerd/internal/spxreport"
)

// A slow route with an SPX capture names where its time goes, one line under
// the route; one without a capture stays a single line.
func TestSlowRoutesShowTheirHottestFunction(t *testing.T) {
	routes := []reqstats.RouteStat{
		{Route: "GET /products/:id", RecentP95Millis: 900, Samples: 12},
		{Route: "GET /", RecentP95Millis: 40, Samples: 30},
	}
	profiles := map[string]spxreport.Profile{
		"GET /products/:id": {Hotspots: []spxreport.Hotspot{{Function: "App\\Models\\Product::price", Pct: 62}, {Function: "PDO::execute", Pct: 20}}},
	}
	out := stripANSI(strings.Join(routesBlock(routes, 80, profiles), "\n"))
	if !strings.Contains(out, "App\\Models\\Product::price") || !strings.Contains(out, "62%") {
		t.Fatalf("expected the hottest function under the slow route:\n%s", out)
	}
	if strings.Contains(out, "PDO::execute") {
		t.Fatalf("only the single hottest function belongs in the list:\n%s", out)
	}
	if strings.Count(out, "\n") != 3 {
		t.Fatalf("header, two routes and one hotspot line expected:\n%s", out)
	}
}

func TestTimingHostsFollowTheSelectedWorktree(t *testing.T) {
	m := NewModel("test")
	s := &siteinfo.EnrichedSite{Name: "shop", Domains: []string{"shop.test", "www.shop.test"},
		Worktrees: []siteinfo.WorktreeInfo{{Branch: "feat", Domain: "feat.shop.test"}}}
	if got := strings.Join(m.timingHosts(s), ","); got != "shop.test,www.shop.test" {
		t.Fatalf("main checkout hosts = %s", got)
	}
	m.timingScope = 1
	if got := strings.Join(m.timingHosts(s), ","); got != "feat.shop.test" {
		t.Fatalf("worktree hosts = %s", got)
	}
}
