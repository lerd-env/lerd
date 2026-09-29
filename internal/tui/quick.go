package tui

import (
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
			continue
		}
		add("Restart worker", a.site+" · "+a.worker, func(m *Model) tea.Cmd { return m.dashFix(a) })
	}
	if len(failingWorkers(m.snap)) > 0 {
		add("Heal crashed workers", "", func(m *Model) tea.Cmd { return m.actionHealWorkers() })
	}

	add("Go to", "Dashboard", func(m *Model) tea.Cmd { m.switchTab(tabDashboard); m.focusMain(); return m.afterNav() })
	add("Go to", "Databases", func(m *Model) tea.Cmd { m.switchTab(tabDatabases); m.focusMain(); return m.afterNav() })
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

	if s := m.currentSite(); s != nil && m.activeTab == tabSites {
		domain := siteDomain(s)
		add("Open in browser", domain, func(m *Model) tea.Cmd { return m.openInBrowserCmd() })
		add("Open in editor", domain, func(m *Model) tea.Cmd { return runLerd(s.Path, "code") })
		add("Open folder", domain, func(m *Model) tea.Cmd { return m.openURL(s.Path) })
	}
	add("Settings", "", func(m *Model) tea.Cmd {
		m.switchTab(tabSites)
		m.detailMode = detailSettings
		m.focusMain()
		return nil
	})
	add("System", "", func(m *Model) tea.Cmd { m.switchTab(tabSites); m.detailMode = detailSystem; m.focusMain(); return nil })
	add("Debug window", "", func(m *Model) tea.Cmd { m.switchTab(tabSites); m.detailMode = detailDumps; m.focusMain(); return nil })
	add("Help", "", func(m *Model) tea.Cmd { m.helpModalActive = true; m.helpScroll = 0; return nil })
	add("Run a lerd command…", ":", func(m *Model) tea.Cmd { m.openPalette(); return nil })
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

// quickScore ranks a candidate against the query: a contiguous match scores by
// where it starts, a scattered one by how far its letters spread. ok is false
// when the query's letters do not all appear in order.
func quickScore(query, text string) (int, bool) {
	q, t := strings.ToLower(strings.TrimSpace(query)), strings.ToLower(text)
	if q == "" {
		return 0, true
	}
	if i := strings.Index(t, q); i >= 0 {
		return i, true
	}
	score, pos := 1000, 0
	for _, r := range q {
		if r == ' ' {
			continue
		}
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
	for _, a := range m.quickActions() {
		if s, ok := quickScore(m.quickQuery, a.label+" "+a.detail); ok {
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
	m.quickActive, m.quickQuery, m.quickCursor = true, "", 0
}

func (m *Model) handleQuickKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	matches := m.quickMatches()
	switch msg.String() {
	case "esc", "ctrl+p":
		m.quickActive = false
	case "ctrl+c":
		m.logTail.Stop()
		return m, tea.Quit
	case "up", "ctrl+k":
		m.quickCursor = max(0, m.quickCursor-1)
	case "down", "ctrl+j":
		m.quickCursor = clamp(m.quickCursor+1, 0, max(0, len(matches)-1))
	case "enter":
		m.quickActive = false
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
