package tui

import (
	"testing"

	"github.com/geodro/lerd/internal/siteinfo"
)

// Two crashed workers on one site are two cards, each restartable on its own.
func TestDashAlerts_OneCardPerCrashedWorker(t *testing.T) {
	m := NewModel("test")
	m.snap = Snapshot{
		Sites:  []siteinfo.EnrichedSite{{Name: "alpha", QueueFailing: true, ScheduleFailing: true}},
		Status: StatusRow{DNSOk: true, NginxRunning: true, WatcherRunning: true},
	}
	alerts := m.dashAlerts()
	if len(alerts) != 2 || alerts[0].worker != "queue" || alerts[1].worker != "schedule" {
		t.Fatalf("expected queue and schedule cards, got %+v", alerts)
	}
}

// Navigating to a site by name must clear a leftover filter that would hide the
// target, so a dashboard click can't silently fail to select.
func TestSelectSiteByName_ClearsHidingFilter(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap() // sites: alpha, beta
	m.siteFilter = "alpha"

	m.selectSiteByName("beta")

	if m.siteFilter != "" {
		t.Fatalf("filter %q hid the target; it should have been cleared", m.siteFilter)
	}
	vis := m.visibleSites()
	if m.siteCursor < 0 || m.siteCursor >= len(vis) || vis[m.siteCursor].Name != "beta" {
		t.Fatalf("cursor %d does not point at beta in %d visible sites", m.siteCursor, len(vis))
	}
}

func TestSelectServiceByName_ClearsHidingFilter(t *testing.T) {
	m := NewModel("test")
	m.snap = fakeSnap() // services: mysql, redis, mailpit
	m.svcFilter = "mysql"

	m.selectServiceByName("redis")

	if m.svcFilter != "" {
		t.Fatalf("filter %q hid the target; it should have been cleared", m.svcFilter)
	}
	vis := m.visibleServices()
	if m.svcCursor < 0 || m.svcCursor >= len(vis) || vis[m.svcCursor].Name != "redis" {
		t.Fatalf("cursor %d does not point at redis in %d visible services", m.svcCursor, len(vis))
	}
}
