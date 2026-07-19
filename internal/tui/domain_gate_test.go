package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/geodro/lerd/internal/siteinfo"
)

// domainGateSnap gives one site with a domain plus a service, so focus can
// legitimately reach the detail pane on either tab.
func domainGateSnap() Snapshot {
	return Snapshot{
		Sites: []siteinfo.EnrichedSite{
			{Name: "alpha", Domains: []string{"alpha.test"}, PHPVersion: "8.3"},
		},
		Services: []ServiceRow{
			{Name: "mysql", State: stateRunning, SiteCount: 1},
		},
		Status: StatusRow{TLD: "test"},
	}
}

// TestDomainActions_GatedToSitesTab guards the regression where add/edit/remove
// domain fired on the carried-over (hidden) site from the Services tab: there
// focus can reach paneDetail while detailMode is still detailSite, so the
// focus+mode guard alone let the mutators touch a site the user can't see.
func TestDomainActions_GatedToSitesTab(t *testing.T) {
	m := NewModel("test")
	m.snap = domainGateSnap()
	m.activeTab = tabServices
	m.detailMode = detailSite
	m.focus = paneDetail
	m.siteCursor = 0
	m.detailCursor = 0 // first navigable row is the domain row

	if m.editFocusedDomain() {
		t.Fatal("editFocusedDomain should not fire off the Sites tab")
	}
	if m.domainInputActive {
		t.Fatal("no domain edit input should open off the Sites tab")
	}
	if handled, _ := m.removeFocusedDomain(); handled {
		t.Fatal("removeFocusedDomain should not fire off the Sites tab")
	}
	if m.confirmActive {
		t.Fatal("no remove-domain confirm should open off the Sites tab")
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: string('a')})
	m = next.(*Model)
	if m.domainInputActive {
		t.Fatal("the `a` add binding should not open input off the Sites tab")
	}
}

// TestDomainActions_WorkOnSitesTab is the positive control: the same actions
// still fire on the Sites tab, so the gate isn't over-broad.
func TestDomainActions_WorkOnSitesTab(t *testing.T) {
	mkModel := func() *Model {
		m := NewModel("test")
		m.snap = domainGateSnap()
		m.activeTab = tabSites
		m.detailMode = detailSite
		m.focus = paneDetail
		m.siteCursor = 0
		m.detailCursor = 0
		return m
	}

	if m := mkModel(); !m.editFocusedDomain() || !m.domainInputActive {
		t.Fatal("editFocusedDomain should open the edit input on the Sites tab")
	}
	if m := mkModel(); func() bool { h, _ := m.removeFocusedDomain(); return h }() == false || !m.confirmActive {
		t.Fatal("removeFocusedDomain should open the confirm on the Sites tab")
	}
	if m := mkModel(); func() bool {
		next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: string('a')})
		return next.(*Model).domainInputActive
	}() == false {
		t.Fatal("the `a` add binding should open the input on the Sites tab")
	}
}
