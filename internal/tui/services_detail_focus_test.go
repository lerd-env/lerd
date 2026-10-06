package tui

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/siteinfo"
)

// TestDetailPane_ServicesTabShowsServiceEvenWhenDetailFocused guards the
// regression where moving focus onto the detail pane on the Services tab fell
// through to rendering the carried-over site's detail instead of the selected
// service.
func TestDetailPane_ServicesTabShowsServiceEvenWhenDetailFocused(t *testing.T) {
	m := NewModel("test")
	m.snap = Snapshot{
		Sites: []siteinfo.EnrichedSite{
			{Name: "alpha", Domains: []string{"alpha.test"}, PHPVersion: "8.3"},
		},
		Services: []ServiceRow{
			{Name: "mysql", State: stateRunning, SiteCount: 1},
		},
		Status: StatusRow{TLD: "test"},
	}
	m.activeTab = tabServices
	m.detailMode = detailSite
	m.focus = paneDetail // focus moved off the list onto the detail pane
	m.siteCursor = 0
	m.svcCursor = 0

	out := stripANSI(m.renderServiceView(80, 24))
	if strings.Contains(out, "alpha.test") {
		t.Fatalf("Services tab detail pane should not render the hidden site:\n%s", out)
	}
	if !strings.Contains(out, "mysql") {
		t.Fatalf("Services tab detail pane should render the selected service:\n%s", out)
	}
}
