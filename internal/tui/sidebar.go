package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/geodro/lerd/internal/siteinfo"
	zone "github.com/lrstanley/bubblezone/v2"
)

// The sidebar is the TUI's one navigation surface: dashboard, databases, sites
// grouped by workspace and services. It carries no selection of its own beyond
// a folded workspace; picking a row sets the same tab and cursors the rest of
// the TUI already reads, so every action keeps working on what is selected.

type sideKind int

const (
	sideDash sideKind = iota
	sideDatabases
	sideWorkspace
	sideSite
	sideService
	sideHeader
	sideBlank
)

type sideItem struct {
	kind sideKind
	key  string
	ws   string // workspace name for sideWorkspace
	idx  int    // index into visibleSites / visibleServices
	text string // header label
}

func (it sideItem) selectable() bool { return it.kind != sideHeader && it.kind != sideBlank }

// sideItems lists the sidebar rows top to bottom. Sites follow visibleSites,
// which already orders them by workspace, so a workspace row is emitted each
// time the owning workspace changes.
func (m *Model) sideItems() []sideItem {
	items := []sideItem{{kind: sideDash, key: "dash"}, {kind: sideDatabases, key: "dbs"}, {kind: sideBlank}}

	sites := m.visibleSites()
	items = append(items, sideItem{kind: sideHeader, key: "h:sites", text: "Sites"})
	of := siteWorkspaces(sites, m.snap.Workspaces)
	current, first := "", true
	for i, s := range sites {
		ws := of[s.Name]
		if ws != current || first {
			if !first {
				items = append(items, sideItem{kind: sideBlank, text: "optional"})
			}
			if ws != "" {
				items = append(items, sideItem{kind: sideWorkspace, key: "ws:" + ws, ws: ws})
			}
			current, first = ws, false
		}
		if ws != "" && m.collapsedWS[ws] {
			continue
		}
		items = append(items, sideItem{kind: sideSite, key: "site:" + s.Name, idx: i})
	}

	items = append(items, sideItem{kind: sideBlank}, sideItem{kind: sideHeader, key: "h:services", text: "Services"})
	for i, s := range m.visibleServices() {
		// Workers show under their site; the sidebar lists real services only.
		if s.WorkerKind != "" {
			continue
		}
		items = append(items, sideItem{kind: sideService, key: "svc:" + s.Name, idx: i})
	}
	return items
}

// sideKeyFromState is the row the rest of the TUI currently has selected.
func (m *Model) sideKeyFromState() string {
	switch m.activeTab {
	case tabDatabases:
		return "dbs"
	case tabSites:
		if s := m.currentSite(); s != nil {
			return "site:" + s.Name
		}
	case tabServices:
		if s := m.currentService(); s != nil {
			if s.WorkerKind != "" {
				return "site:" + s.WorkerSite
			}
			return "svc:" + s.Name
		}
	}
	return "dash"
}

// syncSideKey follows selection made anywhere else. A workspace row is the one
// selection the model has no other record of, so it survives while it exists.
func (m *Model) syncSideKey() {
	if strings.HasPrefix(m.sideKey, "ws:") {
		for _, it := range m.sideItems() {
			if it.key == m.sideKey {
				return
			}
		}
	}
	m.sideKey = m.sideKeyFromState()
}

func (m *Model) sideIndex(items []sideItem) int {
	for i, it := range items {
		if it.key == m.sideKey {
			return i
		}
	}
	return 0
}

// sideMove walks delta selectable rows, stopping at either end.
func (m *Model) sideMove(delta int) {
	m.syncSideKey()
	items := m.sideItems()
	i := m.sideIndex(items)
	step := 1
	if delta < 0 {
		step, delta = -1, -delta
	}
	for ; delta > 0; delta-- {
		j := i + step
		for j >= 0 && j < len(items) && !items[j].selectable() {
			j += step
		}
		if j < 0 || j >= len(items) {
			break
		}
		i = j
	}
	m.sideSelect(items[i])
}

// sideSelect points the rest of the TUI at a sidebar row.
func (m *Model) sideSelect(it sideItem) {
	m.followCursor = true
	m.sideKey = it.key
	switch it.kind {
	case sideDash:
		m.switchTab(tabDashboard)
	case sideDatabases:
		m.switchTab(tabDatabases)
	case sideSite:
		m.switchTab(tabSites)
		if m.siteCursor != it.idx {
			m.siteCursor = it.idx
			m.timingScope = 0 // a new site opens on its own checkout, not a worktree
			m.detailCursor = 0
			m.closePicker()
		}
	case sideService:
		m.switchTab(tabServices)
		if m.svcCursor != it.idx {
			m.svcCursor = it.idx
			m.svcDetailCursor = 0
		}
	}
}

// sideActivate is enter on the sidebar: fold a workspace, or hand focus to the
// main area for everything else.
func (m *Model) sideActivate() {
	m.syncSideKey()
	items := m.sideItems()
	m.sideActivateItem(items[m.sideIndex(items)])
}

func (m *Model) sideActivateItem(it sideItem) {
	m.sideKey = it.key
	if it.kind == sideWorkspace {
		if m.collapsedWS == nil {
			m.collapsedWS = map[string]bool{}
		}
		m.collapsedWS[it.ws] = !m.collapsedWS[it.ws]
		return
	}
	m.sideSelect(it)
	m.focusMain()
}

// focusMain moves keyboard focus off the sidebar onto whatever the main area
// leads with.
func (m *Model) focusMain() {
	m.sideFocus = false
	m.sideOverlay = false
	switch m.activeTab {
	case tabDatabases:
		m.focus = paneDatabases
	case tabSites, tabServices, tabDashboard:
		m.focus = paneDetail
	}
}

// workspaceRollup counts the sites in a workspace that need attention, so a
// folded workspace still shows a crashed worker inside it.
func (m *Model) workspaceRollup(ws string) (failing, paused int) {
	of := siteWorkspaces(m.snap.Sites, m.snap.Workspaces)
	for _, s := range m.snap.Sites {
		if of[s.Name] != ws {
			continue
		}
		switch {
		case siteHasFailingWorker(s):
			failing++
		case s.Paused:
			paused++
		}
	}
	return failing, paused
}

// orderByWorkspace keeps each workspace's sites together in config order,
// preserving the chosen sort inside each group. Sites in no workspace trail.
func orderByWorkspace(sites []siteinfo.EnrichedSite, of map[string]string, rank map[string]int) []siteinfo.EnrichedSite {
	sort.SliceStable(sites, func(i, j int) bool { return rank[of[sites[i].Name]] < rank[of[sites[j].Name]] })
	return sites
}

func siteGlyph(s siteinfo.EnrichedSite) seg {
	switch {
	case siteHasFailingWorker(s):
		return bd(glyphFailing, colFailing)
	case s.Paused:
		return sp(glyphPaused, colPaused)
	case s.FPMRunning:
		return sp(glyphRunning, colRunning)
	}
	return sp(glyphStopped, colDim)
}

func serviceGlyph(st ServiceState) seg {
	switch st {
	case stateRunning:
		return sp(glyphRunning, colRunning)
	case statePaused:
		return sp(glyphPaused, colPaused)
	case stateSuspended:
		return sp(glyphSuspended, colPaused)
	}
	return sp(glyphStopped, colDim)
}

// shortFramework turns "Laravel 12" into "L12" so the tag fits the sidebar.
func shortFramework(label string) string {
	f := strings.Fields(label)
	if len(f) < 2 {
		return label
	}
	return f[0][:1] + f[len(f)-1]
}

func sideLabel(t string) seg { return bd(strings.ToUpper(t), colDim) }

// renderSidebar draws the sidebar as exactly h lines of width w: a fixed top
// (name, dashboard, databases), a scrolling list of sites and services, and
// the core health dots pinned to the bottom.
func (m *Model) renderSidebar(w, h int) []string {
	m.syncSideKey()
	slim := w < 30
	blank := row(surf.s1, w)
	item := func(key string, left, right []seg) string {
		// The sidebar is itself raised, so its selection climbs two steps
		// higher to stand out, and one more with the accent bar when focused.
		bar, bg := sp("  ", nil), surf.s1
		if key == m.sideKey {
			bg = surf.s3
			if m.sideFocus {
				bar, bg = bd("▌ ", colAccent), surf.s4
			}
		}
		line := rowLR(bg, w, append([]seg{bar}, left...), append(right, sp("  ", nil)))
		return zone.Mark("side:"+key, line)
	}

	// A dev build's version carries a git describe suffix; the release part is enough here.
	version, _, _ := strings.Cut(m.version, "-g")
	top := []string{blank, row(surf.s1, w, sp("  ", nil), bd("lerd", colAccent), sp("  "+version, colDim))}
	if m.updateAvailable != "" {
		top = append(top, row(surf.s1, w, sp("  update ", colDim), sp(m.updateAvailable, colAccent), sp(" available", colDim)))
	}
	top = append(top, blank)

	var list []string
	cursorLine := -1
	for _, it := range m.sideItems() {
		if it.key == m.sideKey {
			cursorLine = len(list)
		}
		switch it.kind {
		case sideDash:
			top = append(top, item(it.key, []seg{sp("⌂  ", colDim), bd("Dashboard", nil)}, nil), blank)
		case sideDatabases:
			top = append(top, item(it.key, []seg{sp("≡  ", colDim), bd("Databases", nil)}, nil), blank)
		case sideBlank:
			// Short terminals drop the gaps between workspaces, never the one
			// before a section header.
			if len(list) > 0 && !(it.text == "optional" && layoutFor(m.width, m.height).compact) {
				list = append(list, blank)
			}
		case sideHeader:
			list = append(list, m.sideHeaderRow(it.key, w))
			if f := m.sideFilterRow(it.key, w); f != "" {
				list = append(list, f)
			}
		case sideWorkspace:
			arrow := "▾ "
			if m.collapsedWS[it.ws] {
				arrow = "▸ "
			}
			failing, paused := m.workspaceRollup(it.ws)
			var roll []seg
			switch {
			case failing > 0:
				roll = []seg{bd(fmt.Sprintf("%s %d", glyphFailing, failing), colFailing)}
			case paused > 0:
				roll = []seg{sp(glyphPaused, colPaused)}
			}
			list = append(list, item(it.key, []seg{sp(arrow, colDim), bd(it.ws, nil)}, roll))
		case sideSite:
			s := m.visibleSites()[it.idx]
			name := s.PrimaryDomain()
			if name == "" {
				name = s.Name
			}
			indent := "  "
			if s.GroupSubdomain != "" {
				indent = "  ↳ "
			}
			var tag []seg
			if !slim && s.FrameworkLabel != "" {
				tag = []seg{sp(shortFramework(s.FrameworkLabel), colDim)}
			}
			list = append(list, item(it.key, []seg{sp(indent, nil), siteGlyph(s), sp("  "+name, nil)}, tag))
		case sideService:
			s := m.visibleServices()[it.idx]
			var tag []seg
			if !slim && s.Version != "" {
				tag = []seg{sp(s.Version, colDim)}
			}
			list = append(list, item(it.key, []seg{sp("  ", nil), serviceGlyph(s.State), sp("  "+s.Name, nil)}, tag))
		}
	}
	// Dashboard and Databases sit in the fixed top, outside the scrolling list.
	if m.sideKey == "dash" || m.sideKey == "dbs" {
		cursorLine = -1
	}

	foot := []string{blank, m.sideHealthRow(w, slim), blank}
	listH := max(1, h-len(top)-len(foot))
	follow := -1
	if m.followCursor {
		follow = cursorLine
	}
	visible := viewport(list, follow, listH, &m.sideScroll)
	for len(visible) < listH {
		visible = append(visible, blank)
	}
	out := append(append(top, visible...), foot...)
	return out[:h]
}

func (m *Model) sideHeaderRow(key string, w int) string {
	up, total := 0, 0
	if key == "h:sites" {
		for _, s := range m.snap.Sites {
			total++
			if s.FPMRunning || s.Paused {
				up++
			}
		}
	} else {
		for _, s := range m.snap.Services {
			if s.WorkerKind != "" {
				continue
			}
			total++
			if s.State == stateRunning {
				up++
			}
		}
	}
	label := "Sites"
	if key == "h:services" {
		label = "Services"
	}
	return rowLR(surf.s1, w, []seg{sp("  ", nil), sideLabel(label)}, []seg{sp(fmt.Sprintf("%d/%d  ", up, total), colDim)})
}

// sideFilterRow shows a list's filter while it is being typed or still applies.
func (m *Model) sideFilterRow(key string, w int) string {
	text, pane := m.siteFilter, paneSites
	if key == "h:services" {
		text, pane = m.svcFilter, paneServices
	}
	typing := m.filterActive && m.focus == pane
	if text == "" && !typing {
		return ""
	}
	cursor := ""
	if typing {
		cursor = "▏"
	}
	return row(surf.s1, w, sp("  / ", colAccent), sp(text, nil), sp(cursor, colAccent))
}

func (m *Model) sideHealthRow(w int, slim bool) string {
	dot := func(ok bool) seg {
		if ok {
			return sp(glyphRunning, colRunning)
		}
		return bd(glyphFailing, colFailing)
	}
	gap := "   "
	if slim {
		gap = " "
	}
	st := m.snap.Status
	return row(surf.s1, w, sp("  ", nil), dot(st.DNSOk || st.DNSDisabled), sp(" dns"+gap, colDim), dot(st.NginxRunning), sp(" nginx"+gap, colDim), dot(st.WatcherRunning), sp(" watcher", colDim))
}

// sideWidth is how many columns the sidebar covers right now, counting the
// overlay a narrow terminal opens on demand.
func (m *Model) sideWidth() int {
	if w := layoutFor(m.width, m.height).sideW; w > 0 {
		return w
	}
	if m.sideOverlay {
		return overlaySideW(m.width)
	}
	return 0
}

func overlaySideW(width int) int { return min(34, width-8) }

// focusSidebar hands the arrow keys back to the sidebar and points the list
// focus at the selected section, so s / x / r act on the highlighted row.
func (m *Model) focusSidebar() {
	m.sideFocus = true
	switch m.activeTab {
	case tabSites:
		m.focus = paneSites
	case tabServices:
		m.focus = paneServices
	case tabDatabases:
		m.focus = paneDatabases
	default:
		m.focus = paneDetail
	}
}

// handleSidebarKey owns movement while the sidebar has focus and the few keys
// that move focus between the sidebar and the main area. Anything it does not
// claim falls through to the regular key handling.
func (m *Model) handleSidebarKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	key := msg.String()
	if key == "\\" && layoutFor(m.width, m.height).sideW == 0 {
		m.sideOverlay = !m.sideOverlay
		if m.sideOverlay {
			m.focusSidebar()
		} else {
			m.focusMain()
		}
		return nil, true
	}
	if !m.sideFocus {
		switch key {
		case "tab", "shift+tab":
			// Databases keeps its list and detail side by side in the main area,
			// so tab visits both before returning to the sidebar.
			if m.activeTab == tabDatabases && m.focus == paneDatabases {
				m.focus = paneDetail
				return m.afterNav(), true
			}
			m.focusSidebar()
			return m.afterNav(), true
		case "esc":
			if m.pickerKind == kindInfo && m.detailMode == detailSite && !m.showLogs {
				m.focusSidebar()
				return nil, true
			}
		}
		return nil, false
	}
	switch key {
	case "up", "k":
		m.sideMove(-1)
	case "down", "j":
		m.sideMove(1)
	case "pgup":
		m.sideMove(-10)
	case "pgdown":
		m.sideMove(10)
	case "home", "g":
		m.sideMove(-1 << 20)
	case "end", "G":
		m.sideMove(1 << 20)
	case "enter", "space", "right", "l":
		m.sideActivate()
	case "tab", "shift+tab":
		// Nothing selected means nothing to show, so focus has nowhere to go.
		if (m.activeTab == tabSites && m.currentSite() == nil) || (m.activeTab == tabServices && m.currentService() == nil) {
			return nil, true
		}
		m.focusMain()
	case "esc":
		if !m.sideOverlay {
			return nil, false
		}
		m.sideOverlay = false
	case "/":
		// Filter the section the selection is in; from the dashboard it filters sites.
		if m.activeTab != tabServices && m.activeTab != tabSites {
			m.switchTab(tabSites)
		}
		m.focusSidebar()
		m.filterActive = true
		return nil, true
	default:
		return nil, false
	}
	return m.afterNav(), true
}
