package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/sitedoctor"
)

// stubSiteSweep points the sweep at fixed sites and reports, and restores the
// real lookups when the test ends.
func stubSiteSweep(t *testing.T, sites []config.Site, reports map[string]sitedoctor.Response) {
	t.Helper()
	prevSites, prevReport := sitesForSweep, quickSiteReport
	sitesForSweep = func() []config.Site { return sites }
	quickSiteReport = func(path, _ string) sitedoctor.Response { return reports[path] }
	t.Cleanup(func() { sitesForSweep, quickSiteReport = prevSites, prevReport })
}

// The sweep skips a site the user has ignored and summarises the rest in
// registry order, naming the first failing check so the line says what is wrong.
func TestSweepSites(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", "{}") // gives the site something to check
	sites := []config.Site{
		{Name: "acme", Path: dir, Domains: []string{"acme.test"}},
		{Name: "old", Path: dir, Domains: []string{"old.test"}, Ignored: true},
	}
	stubSiteSweep(t, sites, map[string]sitedoctor.Response{
		dir: {
			Failures: 1, Warnings: 2,
			Checks: []sitedoctor.Check{
				{Name: "node_deps", Status: sitedoctor.StatusWarn},
				{Name: "server_database", Label: "Database", Status: sitedoctor.StatusFail},
			},
		},
	})

	got := sweepSites()
	if len(got) != 1 {
		t.Fatalf("want only the non-ignored site, got %d results", len(got))
	}
	if got[0].Label != "acme.test" {
		t.Errorf("label: got %q, want the primary domain", got[0].Label)
	}
	if !strings.Contains(got[0].Summary, "1 failing") || !strings.Contains(got[0].Summary, "2 warning") {
		t.Errorf("summary %q should carry both counts", got[0].Summary)
	}
	if !strings.Contains(got[0].Summary, "Database") {
		t.Errorf("summary %q should name the failing check, not the warning", got[0].Summary)
	}
}

// A healthy site gets a bare line: no counts, nothing to point at.
func TestSummariseSiteReport_Healthy(t *testing.T) {
	res := summariseSiteReport("acme.test", sitedoctor.Response{})
	if res.Summary != "" || res.Failures != 0 || res.Warnings != 0 {
		t.Errorf("got %+v, want an empty summary", res)
	}
}
