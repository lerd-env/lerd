package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/siteinfo"
)

func statusSite(paused, running bool) siteinfo.EnrichedSite {
	return siteinfo.EnrichedSite{
		Name: "app", Domains: []string{"app.test"}, Path: "/srv/app", PHPVersion: "8.4",
		UsesPHP: true, Paused: paused, FPMRunning: running,
	}
}

// The Status column used to be blank for every site that wasn't paused, so a
// stopped site read the same as a running one.
func TestSiteRowsStatusColumn(t *testing.T) {
	cases := []struct {
		name            string
		paused, running bool
		want            string
	}{
		{"running", false, true, "running"},
		{"stopped", false, false, "stopped"},
		{"paused", true, false, "paused"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := statusSite(c.paused, c.running)
			if got := siteRows(s, false, true)[0][6]; got != c.want {
				t.Errorf("wide Status = %q, want %q", got, c.want)
			}
			if got := siteRows(s, false, false)[0][4]; got != c.want {
				t.Errorf("Status = %q, want %q", got, c.want)
			}
		})
	}
}

// A narrow terminal has no Status column, so only a site that isn't running is
// tagged beside its domain.
func TestPrintSiteCompactTagsOnlySitesNotRunning(t *testing.T) {
	cases := []struct {
		name            string
		paused, running bool
		want            string
	}{
		{"running", false, true, "app.test\n"},
		{"stopped", false, false, "app.test [stopped]\n"},
		{"paused", true, false, "app.test [paused]\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := captureStdout(t, func() { printSiteCompact(statusSite(c.paused, c.running), false) })
			if !strings.HasPrefix(out, c.want) {
				t.Errorf("first line = %q, want %q", strings.SplitN(out, "\n", 2)[0], strings.TrimSuffix(c.want, "\n"))
			}
		})
	}
}

// nginx serves a static site's files without PHP, so a stopped pool on its
// version must not mark it stopped; only nginx being down does.
func TestSiteStatusFollowsNginxForAStaticSite(t *testing.T) {
	prev := sitesNginxRunningFn
	t.Cleanup(func() { sitesNginxRunningFn = prev })

	static := statusSite(false, false)
	static.UsesPHP = false

	sitesNginxRunningFn = func() bool { return true }
	if got := siteStatus(static); got != "running" {
		t.Errorf("static site with nginx up = %q, want running", got)
	}
	sitesNginxRunningFn = func() bool { return false }
	if got := siteStatus(static); got != "stopped" {
		t.Errorf("static site with nginx down = %q, want stopped", got)
	}
	if got := siteStatus(statusSite(false, true)); got != "running" {
		t.Errorf("PHP site with its pool up = %q, want running", got)
	}
}
