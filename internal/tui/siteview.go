package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/geodro/lerd/internal/siteinfo"
	zone "github.com/lrstanley/bubblezone/v2"
)

// The site view is a fixed header (breadcrumb, identity, tab bar) over the
// active tab's content. The header never scrolls, so the site and the tabs
// stay in view however long the Overview or the log tail gets.

var bareFrame = lipgloss.NewStyle()

// renderSitesMain picks what the Sites main area shows: the settings, system
// or debug window when one is open, otherwise the selected site.
func (m *Model) renderSitesMain(w, h int) string {
	if m.detailMode != detailSite {
		titles := map[detailMode]string{detailSettings: "Settings", detailSystem: "System", detailDumps: "Debug"}
		return m.renderFramed(w, h, []string{titles[m.detailMode]}, []seg{sp("esc ", nil), sp("back to the site", colDim)}, zone.Mark("pane:detail", m.renderDetailIn(bareFrame, m.contentWidth(w), h-3, m.focus == paneDetail)))
	}
	if m.currentSite() == nil {
		msg := []string{"No sites linked yet", "cd into a project and run lerd link, or press : and type link"}
		if len(m.snap.Sites) > 0 {
			msg = []string{"No site matches the filter", "press / then esc to clear it"}
		}
		cw := m.contentWidth(w)
		body := strings.Join([]string{row(nil, cw, sp(msg[0], nil)), row(nil, cw, sp(msg[1], colDim))}, "\n")
		return m.renderFramed(w, h, []string{"Sites"}, nil, body)
	}
	return m.renderSiteView(w, h)
}

// contentWidth is the main area's width inside its side margins.
func (m *Model) contentWidth(w int) int {
	return w - 2*max(1, layoutFor(m.width, m.height).padX-1)
}

// renderFramed draws a breadcrumb over body, all inside the main-area margins,
// as exactly h lines of width w.
func (m *Model) renderFramed(w, h int, crumb []string, right []seg, body string) string {
	cw := m.contentWidth(w)
	lines := []string{row(nil, cw), crumbRow(cw, crumb, right), row(nil, cw)}
	return frameLines(w, h, cw, append(lines, strings.Split(body, "\n")...))
}

func (m *Model) renderSiteView(w, h int) string {
	site := m.currentSite()
	cw := m.contentWidth(w)
	head := m.siteHeader(site, cw)
	bodyH := max(3, h-len(head))
	var body string
	if m.siteTab == tabSiteLogs {
		body = zone.Mark("pane:logs", m.renderLogsIn(bareFrame, cw, bodyH, nil))
	} else {
		body = zone.Mark("pane:detail", m.renderDetailIn(bareFrame, cw, bodyH, m.focus == paneDetail && !m.sideFocus))
	}
	return frameLines(w, h, cw, append(head, strings.Split(body, "\n")...))
}

// frameLines pads or cuts lines to h rows of width cw and adds the side margins.
func frameLines(w, h, cw int, lines []string) string {
	pad := (w - cw) / 2
	left, right := row(nil, pad), row(nil, w-cw-pad)
	out := make([]string, h)
	for i := range out {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out[i] = left + padToWidth(clipLine(l, cw), cw) + right
	}
	return strings.Join(out, "\n")
}

func crumbRow(cw int, parts []string, right []seg) string {
	var left []seg
	for i, p := range parts {
		if i > 0 {
			left = append(left, sp("  ›  ", colDim))
		}
		fg := colDim
		if i == len(parts)-1 {
			fg = nil
		}
		left = append(left, sp(p, fg))
	}
	return rowLR(nil, cw, left, right)
}

// siteWorktree is the worktree the site view is scoped to, or nil for the
// site's own checkout. The branch tabs share their position with the timing
// panel's scope, so picking a worktree also shows its traffic.
func (m *Model) siteWorktree(site *siteinfo.EnrichedSite) *siteinfo.WorktreeInfo {
	if site == nil || m.timingScope <= 0 || m.timingScope > len(site.Worktrees) {
		return nil
	}
	return &site.Worktrees[m.timingScope-1]
}

// sitePath is the checkout the site view reads from: the worktree's when one
// is selected.
func (m *Model) sitePath(site *siteinfo.EnrichedSite) string {
	if wt := m.siteWorktree(site); wt != nil {
		return wt.Path
	}
	return site.Path
}

// siteRows are the Overview controls for the selected checkout: the site's
// own rows, or only the selected worktree's.
func (m *Model) siteRows(site *siteinfo.EnrichedSite) []detailRow {
	branch := ""
	if wt := m.siteWorktree(site); wt != nil {
		branch = wt.Branch
	}
	var out []detailRow
	for _, r := range detailRows(site) {
		if r.branch == branch {
			out = append(out, r)
		}
	}
	return out
}

// cycleSiteBranch moves between the site's checkout and its worktrees.
func (m *Model) cycleSiteBranch(delta int) tea.Cmd {
	m.detailCursor, m.detailScroll = 0, 0
	return m.cycleTimingScope(delta)
}

// worktreeView describes a worktree in the site's shape, so the header can
// draw either the same way.
func worktreeView(site *siteinfo.EnrichedSite, wt *siteinfo.WorktreeInfo) *siteinfo.EnrichedSite {
	v := *site
	v.Domains = []string{wt.Domain}
	v.Path = wt.Path
	v.Branch = wt.Branch
	v.LANPort = wt.LANPort
	if wt.PHPVersion != "" {
		v.PHPVersion = wt.PHPVersion
	}
	if wt.NodeVersion != "" {
		v.NodeVersion = wt.NodeVersion
	}
	if wt.FrameworkLabel != "" {
		v.FrameworkLabel = wt.FrameworkLabel
	}
	return &v
}

// siteHeader is the fixed top of the site view.
func (m *Model) siteHeader(parent *siteinfo.EnrichedSite, cw int) []string {
	site, crumb := parent, []string{"Sites", siteDomain(parent)}
	if wt := m.siteWorktree(parent); wt != nil {
		site = worktreeView(parent, wt)
		crumb = append(crumb, wt.Branch)
	}
	domain := siteDomain(site)
	var branch []seg
	if site.Branch != "" && len(parent.Worktrees) == 0 {
		branch = []seg{sp("git ", colDim), sp(site.Branch, nil)}
	}
	out := []string{row(nil, cw), crumbRow(cw, crumb, branch), row(nil, cw)}

	// The URL is the site's name here; showing the bare domain above it too
	// said the same thing twice. The trailing gaps keep the left facts from
	// butting into the right ones when the pane is only just wide enough.
	scheme := "http"
	if site.Secured {
		scheme = "https"
	}
	title := []seg{bd(scheme+"://"+domain, colAccent)}
	if site.AppName != "" && site.AppName != domain {
		title = append(title, sp("   "+site.AppName, colDim))
	}
	// Commands take the registered name, so say it when the domain does not.
	if site == parent && site.Name != "" && !strings.HasPrefix(domain, site.Name+".") {
		title = append(title, sp("   site ", colDim), sp(site.Name, nil))
	}
	out = append(out, rowLR(nil, cw, append(title, sp("   ", nil)), siteVersions(site)))

	var where []seg
	if site.Path != "" {
		home, _ := os.UserHomeDir()
		where = []seg{sp(shortHome(site.Path, home), colDim)}
	}
	out = append(out, rowLR(nil, cw, append(where, sp("   ", nil)), siteFlags(site)))
	if g := siteGroupLine(m, parent); g != "" {
		out = append(out, row(nil, cw, sp(g, colDim)))
	}
	out = append(out, row(nil, cw))
	if tabs := m.siteBranchTabs(parent, cw); len(tabs) > 0 {
		out = append(append(out, tabs...), row(nil, cw))
	}
	return append(out, m.siteTabBar(parent, cw)...)
}

func siteDomain(s *siteinfo.EnrichedSite) string {
	if d := s.PrimaryDomain(); d != "" {
		return d
	}
	return s.Name
}

// siteBranchTabs is one pill per checkout, the site's own first, wrapping onto
// more rows when the branch names do not fit. A worktree whose own worker
// crashed carries the failure mark. Each pill is a click zone.
func (m *Model) siteBranchTabs(site *siteinfo.EnrichedSite, cw int) []string {
	scopes := timingScopes(site)
	if len(scopes) < 2 {
		return nil
	}
	lead := []seg{sp("git  ", colDim)}
	var rows []string
	var line strings.Builder
	line.WriteString(row(nil, segsWidth(lead), lead...))
	used := segsWidth(lead)
	for i, sc := range scopes {
		segs := []seg{sp(" "+sc.label+" ", colDim)}
		if i == m.timingScope {
			segs = []seg{{t: " " + sc.label + " ", bold: true, bg: surf.s3}}
		}
		if i > 0 && worktreeFailing(site.Worktrees[i-1]) {
			segs = append(segs, bd(glyphFailing+" ", colFailing))
		}
		tw := segsWidth(segs) + 1
		if used+tw > cw && used > 5 {
			rows = append(rows, line.String()+row(nil, cw-used))
			line.Reset()
			line.WriteString(row(nil, 5))
			used = 5
		}
		line.WriteString(zone.Mark(fmt.Sprintf("sitebranch:%d", i), row(nil, tw-1, segs...)) + row(nil, 1))
		used += tw
	}
	return append(rows, line.String()+row(nil, max(0, cw-used)))
}

func worktreeFailing(wt siteinfo.WorktreeInfo) bool {
	for _, w := range wt.FrameworkWorkers {
		if w.Failing {
			return true
		}
	}
	return false
}

func siteVersions(site *siteinfo.EnrichedSite) []seg {
	var out []seg
	add := func(label, value string) {
		if value == "" {
			return
		}
		if len(out) > 0 {
			out = append(out, sp("   ", nil))
		}
		if label != "" {
			out = append(out, sp(label+" ", colDim))
		}
		out = append(out, sp(value, nil))
	}
	add("", site.FrameworkLabel)
	php := site.PHPVersion
	if php == "" && site.ContainerPort > 0 {
		php = "custom"
	}
	add("php", php)
	add("node", site.NodeVersion)
	if site.Runtime == "frankenphp" {
		rt := "frankenphp"
		if site.RuntimeWorker {
			rt += " worker"
		}
		add("runtime", rt)
	}
	return out
}

func siteFlags(site *siteinfo.EnrichedSite) []seg {
	flag := func(on bool, label string) []seg {
		if on {
			return []seg{sp(glyphRunning+" ", colRunning), sp(label, colDim)}
		}
		return []seg{sp(glyphStopped+" ", colDim), sp(label, colDim)}
	}
	out := flag(site.Secured, "https")
	out = append(out, sp("     ", nil))
	out = append(out, flag(site.LANPort > 0, "lan")...)
	if site.Paused {
		out = append(append([]seg{sp(glyphPaused+" ", colPaused), sp("paused", colPaused)}, sp("     ", nil)), out...)
	}
	return out
}

// siteTabBar is the tab labels over a hairline the active tab thickens in the
// accent. Each label is a click zone that selects its tab.
func (m *Model) siteTabBar(site *siteinfo.EnrichedSite, cw int) []string {
	var labels strings.Builder
	x, ax, aw := 0, 0, 0
	for i, t := range availableSiteTabs(site) {
		name := siteTabLabel(t)
		if i > 0 {
			labels.WriteString(row(nil, 4))
			x += 4
		}
		label := row(nil, len(name), sp(name, colDim))
		if t == m.siteTab {
			label, ax, aw = row(nil, len(name), bd(name, nil)), x, len(name)
		}
		labels.WriteString(zone.Mark(fmt.Sprintf("sitetab:%d", i), label))
		x += len(name)
	}
	line := row(nil, cw, sp(strings.Repeat("─", ax), colDivider), sp(strings.Repeat("━", aw), colAccent), sp(strings.Repeat("─", max(0, cw-ax-aw)), colDivider))
	return []string{padToWidth(labels.String(), cw), line, row(nil, cw)}
}

// shortHome writes a path under the user's home as ~/…, which is how people
// read their own project paths.
func shortHome(path, home string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+"/")) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
