package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/geodro/lerd/internal/siteinfo"
)

func siteViewModel() *Model {
	m := NewModel("test")
	m.width, m.height = 160, 45
	m.snap = Snapshot{
		Sites: []siteinfo.EnrichedSite{{
			Name: "shop", Domains: []string{"shop.test"}, Path: "/home/dev/Code/shop", Secured: true,
			FPMRunning: true, PHPVersion: "8.4", NodeVersion: "22", FrameworkLabel: "Laravel 12", Branch: "main",
		}},
		Status: StatusRow{TLD: "test", DNSOk: true, NginxRunning: true, WatcherRunning: true},
	}
	m.switchTab(tabSites)
	m.focusMain()
	return m
}

func TestSiteViewFitsItsBox(t *testing.T) {
	m := siteViewModel()
	lines := strings.Split(m.renderSiteView(120, 40), "\n")
	if len(lines) != 40 {
		t.Fatalf("site view has %d lines, want 40", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 120 {
			t.Fatalf("line %d is %d wide, want 120: %q", i, w, ansi.Strip(l))
		}
	}
}

func TestSiteViewHeaderCarriesTheSiteFacts(t *testing.T) {
	m := siteViewModel()
	out := ansi.Strip(m.renderSiteView(140, 40))
	for _, want := range []string{"Sites  ›  shop.test", "https://shop.test", "Laravel 12", "php 8.4", "node 22", "Code/shop", "main"} {
		if !strings.Contains(out, want) {
			t.Errorf("header missing %q:\n%s", want, out)
		}
	}
	for _, tab := range []string{"Overview", "Logs", "Env", "Debug", "Doctor"} {
		if !strings.Contains(out, tab) {
			t.Errorf("tab bar missing %q", tab)
		}
	}
}

// The header now owns the identity and the tab strip, so the Overview body
// must not repeat either.
func TestSiteOverviewDoesNotRepeatTheHeader(t *testing.T) {
	m := siteViewModel()
	out := ansi.Strip(m.renderSiteView(140, 40))
	if strings.Contains(out, "[1] Overview") {
		t.Errorf("old bracketed tab strip still rendered:\n%s", out)
	}
	if strings.Count(out, "/home/dev/Code/shop")+strings.Count(out, "~/Code/shop") != 1 {
		t.Errorf("the path should appear once, in the header:\n%s", out)
	}
}

func TestSiteViewSettingsModeShowsItsOwnTitle(t *testing.T) {
	m := siteViewModel()
	m.detailMode = detailSettings
	out := ansi.Strip(m.renderBody(120, 40))
	if !strings.Contains(out, "Settings") || strings.Contains(out, "Overview") {
		t.Fatalf("settings should replace the site view:\n%s", out)
	}
}

func TestSitesWithoutSelectionShowAnEmptyState(t *testing.T) {
	m := NewModel("test")
	m.width, m.height = 120, 40
	m.switchTab(tabSites)
	out := ansi.Strip(m.renderBody(100, 30))
	if !strings.Contains(out, "No sites linked yet") {
		t.Fatalf("expected an empty state:\n%s", out)
	}
}

func TestShortHomePath(t *testing.T) {
	if got := shortHome("/home/dev/Code/shop", "/home/dev"); got != "~/Code/shop" {
		t.Fatalf("shortHome = %q", got)
	}
	if got := shortHome("/srv/shop", "/home/dev"); got != "/srv/shop" {
		t.Fatalf("paths outside home stay absolute, got %q", got)
	}
}

// The URL names the site; the bare domain above it said the same thing twice.
func TestSiteHeaderNamesTheSiteOnce(t *testing.T) {
	m := siteViewModel()
	head := m.siteHeader(m.currentSite(), 140)
	identity := ansi.Strip(strings.Join(head[3:], "\n")) // below the breadcrumb
	if n := strings.Count(identity, "shop.test"); n != 1 {
		t.Fatalf("the domain should appear once under the breadcrumb, got %d:\n%s", n, identity)
	}
}

func TestSiteOverviewNamesEachWorkersUnit(t *testing.T) {
	m := siteViewModel()
	m.snap.Sites[0].HasQueueWorker = true
	m.snap.Sites[0].QueueRunning = true
	if out := ansi.Strip(m.renderSiteView(160, 45)); !strings.Contains(out, "lerd-queue-shop") {
		t.Fatalf("the Overview's queue row leaves out its unit:\n%s", out)
	}
}

func TestSiteOverviewShowsLongWorkerUnitsWhole(t *testing.T) {
	m := siteViewModel()
	s := &m.snap.Sites[0]
	s.Name, s.Domains = "customer-portal-backend", []string{"customer-portal-backend.test"}
	s.HasQueueWorker, s.HasScheduleWorker = true, true
	out := ansi.Strip(m.renderSiteView(140, 60))
	for _, unit := range []string{"lerd-queue-customer-portal-backend", "lerd-schedule-customer-portal-backend"} {
		if !strings.Contains(out, unit) {
			t.Errorf("the Overview cuts %s short:\n%s", unit, out)
		}
	}
}

func TestSiteHeaderNamesASiteWhoseNameIsNotItsDomain(t *testing.T) {
	m := siteViewModel()
	if out := ansi.Strip(strings.Join(m.siteHeader(m.currentSite(), 140), "\n")); strings.Contains(out, "site shop") {
		t.Fatalf("a site named after its domain should not repeat the name:\n%s", out)
	}
	m.snap.Sites[0].Name = "crm"
	if out := ansi.Strip(strings.Join(m.siteHeader(m.currentSite(), 140), "\n")); !strings.Contains(out, "site crm") {
		t.Fatalf("the header should name a site registered as crm on shop.test:\n%s", out)
	}
}
