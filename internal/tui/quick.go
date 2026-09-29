package tui

import (
	"github.com/geodro/lerd/internal/siteinfo"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The ctrl+p palette: one fuzzy list of places to go and reversible things to
// do, drawn over the dimmed screen. Anything destructive stays in the CLI, and
// "Run a lerd command" hands over to the : prompt for the rest.

type quickAction struct {
	label, detail string
	run           func(m *Model) tea.Cmd
}

const quickMaxRows = 12

func (m *Model) quickActions() []quickAction {
	var out []quickAction
	add := func(label, detail string, run func(m *Model) tea.Cmd) {
		out = append(out, quickAction{label, detail, run})
	}

	// Fixes first: they are why someone opens the palette in a hurry.
	for _, a := range m.dashAlerts() {
		a := a
		if a.site == "" {
			add("Start lerd", a.title, func(m *Model) tea.Cmd { return m.dashFix(a) })
			continue
		}
		add("Restart worker", a.site+" · "+a.worker, func(m *Model) tea.Cmd { return m.dashFix(a) })
	}
	if len(failingWorkers(m.snap)) > 0 {
		add("Heal crashed workers", "", func(m *Model) tea.Cmd { return m.actionHealWorkers() })
	}

	out = append(out, m.contextActions()...)
	out = append(out, m.placeActions()...)
	out = append(out, m.settingsActions()...)

	for _, svc := range m.snap.Services {
		if svc.WorkerKind != "" {
			continue
		}
		name := svc.Name
		verbs := []string{"start"}
		if svc.State == stateRunning {
			verbs = []string{"stop", "restart"}
		}
		for _, verb := range verbs {
			verb := verb
			add(strings.ToUpper(verb[:1])+verb[1:]+" service", name, func(m *Model) tea.Cmd {
				m.setStatus(verb+"ing "+name+"…", 10*time.Second)
				return tea.Sequence(runLerd("", "service", verb, name), loadCmd())
			})
		}
	}
	add("Add a service preset", "", func(m *Model) tea.Cmd { m.openPaletteIn("", "service preset "); return nil })
	add("Run a lerd command…", ":", func(m *Model) tea.Cmd { m.openPalette(); return nil })
	return out
}

// pressKey runs a view's own key handler, so a palette entry and its shortcut
// can never drift apart. Focus moves to the main area first, as the key expects.
func pressKey(r rune) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		m.focusMain()
		_, cmd := m.handleMainKey(tea.KeyPressMsg{Code: r, Text: string(r)})
		return cmd
	}
}

// contextActions offer every action on every site, service, database and
// runtime, the selected one's first. Each entry selects its target before
// acting, so nothing has to be opened before it can be done.
func (m *Model) contextActions() []quickAction {
	var mine, rest []quickAction
	cur := m.currentSite()
	for i := range m.snap.Sites {
		s := &m.snap.Sites[i]
		acts := m.siteActions(s)
		if m.activeTab == tabSites && cur != nil && cur.Name == s.Name {
			mine = append(mine, acts...)
		} else {
			rest = append(rest, acts...)
		}
	}
	curSvc := m.currentService()
	for i := range m.snap.Services {
		svc := &m.snap.Services[i]
		if svc.WorkerKind != "" {
			continue
		}
		acts := m.serviceActions(svc)
		if m.activeTab == tabServices && curSvc != nil && curSvc.Name == svc.Name {
			mine = append(mine, acts...)
		} else {
			rest = append(rest, acts...)
		}
	}
	dbActs := m.databaseActions()
	rtActs := m.runtimeActions()
	switch m.activeTab {
	case tabDatabases:
		mine = append(mine, dbActs...)
	case tabRuntimes:
		mine = append(mine, rtActs...)
	default:
		rest = append(append(rest, dbActs...), rtActs...)
	}
	if m.activeTab == tabCore {
		mine = append(mine, quickAction{"Start lerd", m.coreName, pressKey('s')})
	}
	return append(mine, rest...)
}

// onSite wraps an action so it first opens the site, scoped to branch ("" for
// its own checkout).
func onSite(name, branch string, f func(m *Model) tea.Cmd) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		m.switchTab(tabSites)
		m.selectSiteByName(name)
		m.detailMode = detailSite
		m.timingScope = 0
		if s := m.currentSite(); s != nil {
			for i, wt := range s.Worktrees {
				if wt.Branch == branch {
					m.timingScope = i + 1
				}
			}
		}
		m.focusMain()
		return tea.Batch(m.afterNav(), f(m))
	}
}

func (m *Model) siteActions(s *siteinfo.EnrichedSite) []quickAction {
	var out []quickAction
	d, name := siteDomain(s), s.Name
	add := func(label string, f func(m *Model) tea.Cmd) {
		out = append(out, quickAction{label, d, onSite(name, "", f)})
	}
	pause := "Pause site"
	if s.Paused {
		pause = "Resume site"
	}
	add("Restart site", func(m *Model) tea.Cmd { return m.actionRestart() })
	add(pause, func(m *Model) tea.Cmd { return m.actionPauseToggle() })
	add("Open shell", func(m *Model) tea.Cmd { return m.actionShell() })
	add("Open in browser", func(m *Model) tea.Cmd { return m.openInBrowserCmd() })
	add("Open in editor", pressKey('E'))
	add("Open folder", pressKey('F'))
	add("New worktree", pressKey('W'))
	for i, t := range availableSiteTabs(s) {
		n := i + 1
		add("Show "+siteTabLabel(t), func(m *Model) tea.Cmd { return m.selectSiteTab(n) })
	}
	for _, r := range detailRows(s) {
		r := r
		label := siteToggleLabel(r)
		if label == "" {
			continue
		}
		detail := d
		if r.branch != "" {
			detail = r.branch + " · " + d
		}
		out = append(out, quickAction{label, detail, onSite(name, r.branch, func(m *Model) tea.Cmd { return m.toggleSiteRow(r) })})
	}
	return out
}

func (m *Model) serviceActions(svc *ServiceRow) []quickAction {
	var out []quickAction
	name := svc.Name
	add := func(label string, f func(m *Model) tea.Cmd) {
		out = append(out, quickAction{label, name, func(m *Model) tea.Cmd {
			m.switchTab(tabServices)
			m.selectServiceByName(name)
			m.focusMain()
			return tea.Batch(m.afterNav(), f(m))
		}})
	}
	pin := "Pin service"
	if svc.Pinned {
		pin = "Unpin service"
	}
	add(pin, pressKey('P'))
	add("Update service", func(m *Model) tea.Cmd { return m.actionServiceUpdate() })
	add("Roll back service", func(m *Model) tea.Cmd { return m.actionServiceRollback() })
	add("Open shell", func(m *Model) tea.Cmd { return m.actionShell() })
	if svc.Dashboard != "" {
		add("Open service dashboard", func(m *Model) tea.Cmd { return m.openServiceDashboardCmd() })
	}
	add("Show Overview", func(m *Model) tea.Cmd { return m.selectServiceTab(svcTabOverview) })
	add("Show Logs", func(m *Model) tea.Cmd { return m.selectServiceTab(svcTabLogs) })
	return out
}

func (m *Model) databaseActions() []quickAction {
	var out []quickAction
	rows := m.dbRows()
	for pos, i := range navigableDBRows(rows) {
		pos, r := pos, rows[i]
		db := m.dbEngines[r.engine].Databases[r.database]
		add := func(label string, f func(m *Model) tea.Cmd) {
			out = append(out, quickAction{label, db.Name, func(m *Model) tea.Cmd {
				m.switchTab(tabDatabases)
				m.dbCursor = pos
				m.focusMain()
				m.focus = paneDatabases
				return f(m)
			}})
		}
		add("Snapshot database", func(m *Model) tea.Cmd { return m.actionDatabaseSnapshot() })
		add("Export database", pressKey('e'))
		add("Include or exclude from auto snapshots", pressKey('a'))
	}
	out = append(out, quickAction{"Create a database", "", func(m *Model) tea.Cmd {
		m.switchTab(tabDatabases)
		return pressKey('c')(m)
	}})
	return out
}

func (m *Model) runtimeActions() []quickAction {
	var out []quickAction
	for i, r := range m.runtimeRows() {
		i := i
		name := map[string]string{"php": "PHP ", "node": "Node "}[r.kind] + r.version
		add := func(label string, key rune) {
			out = append(out, quickAction{label, name, func(m *Model) tea.Cmd {
				m.switchTab(tabRuntimes)
				m.rtCursor = i
				return pressKey(key)(m)
			}})
		}
		add("Make default", 'd')
		if r.kind == "php" {
			add("Toggle Xdebug", 'x')
			add("Rebuild", 'R')
		}
	}
	out = append(out,
		quickAction{"Install a PHP version", "", func(m *Model) tea.Cmd { m.openPaletteIn("", "use "); return nil }},
		quickAction{"Install a Node version", "", func(m *Model) tea.Cmd { m.openPaletteIn("", "node:install "); return nil }})
	return out
}

// siteToggleLabel names a site Overview control for the palette, or "" for a
// row the palette does not offer (a domain, an info line).
func siteToggleLabel(r detailRow) string {
	switch r.kind {
	case kindHTTPS:
		return "Toggle HTTPS"
	case kindLANShare:
		return "Toggle LAN share"
	case kindPin:
		return "Toggle keep awake"
	case kindRuntime:
		return "Switch runtime (php-fpm / FrankenPHP)"
	case kindHorizonReload:
		return "Toggle Horizon reload"
	case kindStripe:
		return "Toggle Stripe listener"
	case kindAutoSnapshot:
		return "Change auto snapshots"
	case kindPHP:
		return "Change PHP version"
	case kindNode:
		return "Change Node version"
	case kindDomainAdd:
		return "Add a domain"
	case kindWorker, kindWorktreeWorker:
		return "Start or stop worker " + r.workerName
	case kindWorktreeDB:
		return "Toggle isolated database"
	case kindWorktreeLAN:
		return "Toggle worktree LAN share"
	}
	return ""
}

// toggleSiteRow points the Overview cursor at a row and toggles it, the same
// path space takes on that row.
func (m *Model) toggleSiteRow(r detailRow) tea.Cmd {
	s := m.currentSite()
	rows := m.siteRows(s)
	nav := navigableRows(rows)
	for pos, i := range nav {
		if rows[i] == r {
			m.detailCursor = pos
			m.focusMain()
			return m.detailToggleSelected(s, rows, nav)
		}
	}
	return nil
}

// placeActions reach every page and every site, worktree and service.
func (m *Model) placeActions() []quickAction {
	var out []quickAction
	add := func(label, detail string, run func(m *Model) tea.Cmd) {
		out = append(out, quickAction{label, detail, run})
	}
	page := func(name string, tab topTab, mode detailMode) {
		add("Go to", name, func(m *Model) tea.Cmd {
			m.switchTab(tab)
			m.detailMode = mode
			m.focusMain()
			return m.afterNav()
		})
	}
	page("Dashboard", tabDashboard, detailSite)
	page("Databases", tabDatabases, detailSite)
	page("PHP & Node", tabRuntimes, detailSite)
	page("Settings", tabSites, detailSettings)
	page("System", tabSites, detailSystem)
	page("Debug window", tabSites, detailDumps)
	for _, c := range coreProcesses {
		name := c.name
		add("Go to", name, func(m *Model) tea.Cmd {
			m.switchTab(tabCore)
			m.coreName = name
			m.focusMain()
			return m.afterNav()
		})
	}
	add("Help", "", func(m *Model) tea.Cmd { m.helpModalActive = true; m.helpScroll = 0; return nil })
	for _, s := range m.snap.Sites {
		name, domain := s.Name, siteDomain(&s)
		add("Open site", domain, func(m *Model) tea.Cmd { return m.quickOpenSite(name, 0) })
		for i, wt := range s.Worktrees {
			scope := i + 1
			add("Open worktree", wt.Branch+" · "+domain, func(m *Model) tea.Cmd { return m.quickOpenSite(name, scope) })
		}
	}
	for _, svc := range m.snap.Services {
		if svc.WorkerKind != "" {
			continue
		}
		name := svc.Name
		add("Open service", name, func(m *Model) tea.Cmd {
			m.switchTab(tabServices)
			m.selectServiceByName(name)
			m.focusMain()
			return m.afterNav()
		})
	}
	return out
}

// settingsActions offer every Settings toggle, named for what it does now.
func (m *Model) settingsActions() []quickAction {
	var out []quickAction
	for i, r := range m.settingsRows() {
		i := i
		verb := "Turn on"
		if r.on {
			verb = "Turn off"
		}
		out = append(out, quickAction{verb, r.label, func(m *Model) tea.Cmd {
			rows := m.settingsRows()
			m.settingsRow = i
			return m.settingsToggle(rows)
		}})
	}
	return out
}

func (m *Model) quickOpenSite(name string, scope int) tea.Cmd {
	m.switchTab(tabSites)
	m.selectSiteByName(name)
	m.detailMode = detailSite
	m.siteTab = tabSiteOverview
	m.timingScope = scope
	m.detailCursor, m.detailScroll = 0, 0
	m.focusMain()
	return m.afterNav()
}

// quickScore ranks a candidate against the query. Each word of the query is
// matched on its own, in any order, so "drupal https" finds what "https drupal"
// does; every word must match. A word found whole scores by where it starts, one
// whose letters are only scattered in order scores by how far they spread.
func quickScore(query, text string) (int, bool) {
	t := strings.ToLower(text)
	total := 0
	for _, word := range strings.Fields(strings.ToLower(query)) {
		s, ok := wordScore(word, t)
		if !ok {
			return 0, false
		}
		total += s
	}
	return total, true
}

func wordScore(word, t string) (int, bool) {
	if i := strings.Index(t, word); i >= 0 {
		return i, true
	}
	score, pos := 1000, 0
	for _, r := range word {
		i := strings.IndexRune(t[pos:], r)
		if i < 0 {
			return 0, false
		}
		score += i
		pos += i + 1
	}
	return score, true
}

func (m *Model) quickMatches() []quickAction {
	type scored struct {
		a     quickAction
		score int
	}
	var hits []scored
	// Built once per opening: the list covers every site, service, database
	// and setting, which is too much to rebuild on each keystroke and frame.
	if m.quickCache == nil {
		m.quickCache = m.quickActions()
	}
	for _, a := range m.quickCache {
		if s, ok := quickScore(m.quickQuery, a.label+" "+a.detail); ok {
			// Typing a name should reach the thing before everything done to it.
			if a.label == "Go to" || strings.HasPrefix(a.label, "Open site") || strings.HasPrefix(a.label, "Open worktree") || strings.HasPrefix(a.label, "Open service") {
				s -= 20
			}
			hits = append(hits, scored{a, s})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score < hits[j].score })
	out := make([]quickAction, len(hits))
	for i, h := range hits {
		out[i] = h.a
	}
	return out
}

func (m *Model) openQuick() {
	m.quickActive, m.quickQuery, m.quickCursor, m.quickCache = true, "", 0, nil
}

func (m *Model) handleQuickKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	matches := m.quickMatches()
	switch msg.String() {
	case "esc", "ctrl+p":
		m.quickActive, m.quickCache = false, nil
	case "ctrl+c":
		m.logTail.Stop()
		return m, tea.Quit
	case "up", "ctrl+k":
		m.quickCursor = max(0, m.quickCursor-1)
	case "down", "ctrl+j":
		m.quickCursor = clamp(m.quickCursor+1, 0, max(0, len(matches)-1))
	case "enter":
		m.quickActive, m.quickCache = false, nil
		if m.quickCursor < len(matches) {
			return m, matches[m.quickCursor].run(m)
		}
	case "backspace":
		if r := []rune(m.quickQuery); len(r) > 0 {
			m.quickQuery = string(r[:len(r)-1])
			m.quickCursor = 0
		}
	default:
		if msg.Text != "" {
			m.quickQuery += msg.Text
			m.quickCursor = 0
		}
	}
	return m, nil
}

// withQuickOverlay dims the screen and draws the palette box over it.
func (m *Model) withQuickOverlay(lines []string) []string {
	for i, l := range lines {
		lines[i] = dimStyle.Render(ansi.Strip(l))
	}
	pw := min(72, m.width-6)
	matches := m.quickMatches()
	m.quickCursor = clamp(m.quickCursor, 0, max(0, len(matches)-1))

	box := []string{row(surf.s3, pw)}
	box = append(box, rowLR(surf.s3, pw, []seg{sp("  ", nil), bd("Go to or do", nil)}, []seg{sp("esc  ", colDim)}))
	box = append(box, row(surf.s3, pw))
	field := func(segs ...seg) string { return row(surf.s3, 2) + row(surf.s4, pw-4, segs...) + row(surf.s3, 2) }
	input := field(sp("  › ", colAccent), sp(m.quickQuery, nil), sp("▏", colAccent))
	if m.quickQuery == "" {
		input = field(sp("  › ", colAccent), sp("▏", colAccent), sp("sites, services, worktrees, actions", colDim))
	}
	box = append(box, field(), input, field(), row(surf.s3, pw))

	start := max(0, m.quickCursor-quickMaxRows+1)
	for i := start; i < len(matches) && i < start+quickMaxRows; i++ {
		a := matches[i]
		if i == m.quickCursor {
			box = append(box, row(surf.s3, 2)+rowLR(surf.s4, pw-4, []seg{bd("▌ ", colAccent), bd(padRight(a.label, 22), nil), sp(a.detail, colDim)}, []seg{sp("↵  ", colDim)})+row(surf.s3, 2))
			continue
		}
		box = append(box, row(surf.s3, 2)+row(surf.s3, pw-4, sp("  "+padRight(a.label, 22), nil), sp(a.detail, colDim))+row(surf.s3, 2))
	}
	if len(matches) == 0 {
		box = append(box, row(surf.s3, pw, sp("    nothing matches", colDim)))
	}
	for len(box) < quickMaxRows+8 {
		box = append(box, row(surf.s3, pw))
	}
	box = append(box, rowLR(surf.s3, pw, []seg{sp("  ↑↓ ", nil), sp("select   ", colDim), sp("enter ", nil), sp("run", colDim)}, nil), row(surf.s3, pw))

	x := (m.width - pw) / 2
	y := max(1, (m.height-len(box))/3)
	for i, b := range box {
		if y+i < len(lines) {
			lines[y+i] = splice(padToWidth(lines[y+i], m.width), b, x)
		}
	}
	return lines
}
