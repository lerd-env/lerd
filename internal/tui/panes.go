package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteinfo"
	zone "github.com/lrstanley/bubblezone"
)

// narrowWidth is the terminal width below which the TUI switches from the
// two-column layout (sites+services | detail) to a single-column layout
// where only the focused pane fills the full width.
const narrowWidth = 100

// View implements tea.Model.
func (m *Model) View() string {
	if m.width < 60 || m.height < 12 {
		return "terminal too small (need at least 60×12)\n"
	}

	// When a modal is open, return a full-screen centered overlay instead
	// of the base layout. Less ambient context but consistent with the
	// existing detail-pane swap pattern (S / Y / D / F / ?) which already
	// replaces the right column wholesale. Toasts still composite on top
	// so a completing action result isn't silently lost while a modal
	// (palette / confirm / picker / help) is open.
	if m.modalActive() {
		toasts := m.renderToasts(m.width)
		modalH := m.height - lipgloss.Height(toasts)
		if modalH < 6 {
			modalH = m.height
			toasts = ""
		}
		out := m.renderActiveModal(m.width, modalH)
		if toasts != "" {
			out = lipgloss.JoinVertical(lipgloss.Left, out, toasts)
		}
		return out
	}

	tabs := m.renderTabs(m.width)
	footer := m.renderFooter()
	statusBar := m.renderStatus()

	// Toasts float over the content as an overlay rather than claiming layout
	// rows, so a transient notification never reflows the panes underneath.
	reserved := lipgloss.Height(tabs) + lipgloss.Height(footer)
	if statusBar != "" {
		reserved += lipgloss.Height(statusBar)
	}
	bodyH := m.height - reserved
	if bodyH < 6 {
		bodyH = 6
	}

	// The full-width logs pane is the manual `l` toggle. On the Services tab the
	// logs live in a sub-pane inside the detail column instead, so the
	// full-width one steps aside to avoid showing the tail twice.
	showFullLogs := m.showLogs && !m.serviceLogsActive()

	// Logs pane takes at least half the window when open, and can grow larger,
	// leaving only a sliver of the top pane so the log view dominates.
	logH := 0
	if showFullLogs {
		logH = bodyH / 2
		if h := m.height / 2; h > logH {
			logH = h
		}
		if logH < 10 {
			logH = 10
		}
		if logH > bodyH-4 {
			logH = bodyH - 4
		}
	}
	topH := bodyH - logH
	if topH < 4 {
		topH = 4
	}

	top := m.renderBody(topH)

	sections := []string{tabs, top}
	if showFullLogs {
		sections = append(sections, zone.Mark("pane:logs", m.renderLogs(m.width, logH)))
	}
	if statusBar != "" {
		sections = append(sections, statusBar)
	}
	sections = append(sections, footer)

	out := lipgloss.JoinVertical(lipgloss.Left, sections...)
	// Composite toasts over the bottom-right, just above the footer, without
	// having reserved any rows for them above.
	if stack := m.toastStack(); stack != "" {
		out = overlayBottomRight(out, stack, lipgloss.Height(footer))
	}
	return zone.Scan(out)
}

// overlayBottomRight paints overlay onto base anchored to the right edge, with
// its last line marginBottom rows above the bottom of base. Each overlay line
// is right-aligned individually and only the cells it covers are overwritten,
// so content to the left of the overlay shows through.
func overlayBottomRight(base, overlay string, marginBottom int) string {
	baseLines := strings.Split(base, "\n")
	ovLines := strings.Split(overlay, "\n")
	start := len(baseLines) - marginBottom - len(ovLines)
	if start < 0 {
		start = 0
	}
	for i, ol := range ovLines {
		row := start + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		olW := ansi.StringWidth(ol)
		col := ansi.StringWidth(baseLines[row]) - olW
		if col < 0 {
			col = 0
		}
		left := padToWidth(ansi.Truncate(baseLines[row], col, ""), col)
		baseLines[row] = left + ol
	}
	return strings.Join(baseLines, "\n")
}

// renderTabs draws the clickable top tab strip. Each label is wrapped in a
// bubblezone mark ("tab:<label>") so handleMouse can hit-test a click without
// tracking column offsets by hand; the active tab reads as a filled accent
// pill, the rest sit dim.
func (m *Model) renderTabs(width int) string {
	parts := make([]string, 0, len(orderedTabs))
	for _, t := range orderedTabs {
		style := tabInactiveStyle
		if t == m.activeTab {
			style = tabActiveStyle
		}
		parts = append(parts, zone.Mark("tab:"+t.label(), style.Render(t.label())))
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Top, parts...)

	// The version sits on the far right of the same row; an update banner
	// follows it in accent when a newer release is available.
	right := titleStyle.Render("lerd " + m.version)
	if m.updateAvailable != "" {
		right += "  " + accentStyle.Render("update "+m.updateAvailable)
	}
	inner := width - 2 // tabBarStyle horizontal padding
	gap := inner - lipgloss.Width(bar) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return tabBarStyle.Render(bar + spaces(gap) + right)
}

// renderBody renders the active tab's screen into the given height: the
// six-card dashboard grid, the sites list + site detail, or the services list
// + service detail. Sites/Services reuse the wide/narrow split that the old
// combined layout used, minus the second list pane.
func (m *Model) renderBody(topH int) string {
	if m.activeTab == tabDashboard {
		return m.renderDashboardGrid(m.width, topH)
	}

	listPane := m.renderSites
	listZone := "pane:sites"
	if m.activeTab == tabServices {
		listPane = m.renderServices
		listZone = "pane:services"
	}

	if m.width < narrowWidth {
		// Narrow: stack the list on top, detail below.
		listH := topH * 2 / 5
		if listH < 6 {
			listH = 6
		}
		if listH > topH-6 {
			listH = topH - 6
		}
		detailH := topH - listH

		// Settings / system / dumps take the full height so the content isn't
		// cramped between the list and a slim detail pane (Sites tab only).
		if m.activeTab == tabSites && (m.detailMode == detailSettings || m.detailMode == detailSystem || m.detailMode == detailDumps) {
			return zone.Mark("pane:detail", m.renderDetailInline(m.width, topH, true))
		}
		list := zone.Mark(listZone, listPane(m.width, listH))
		detail := m.renderDetailColumn(m.width, detailH, m.focus == paneDetail)
		return lipgloss.JoinVertical(lipgloss.Left, list, detail)
	}

	// Wide: list on the left, detail on the right. The lists are slim (status
	// dot, name, short meta), so they take about a quarter of the width and are
	// capped so they never sprawl on a wide terminal — the detail gets the rest.
	leftW := m.width / 4
	if leftW < 28 {
		leftW = 28
	}
	if leftW > 46 {
		leftW = 46
	}
	if leftW > m.width-30 {
		leftW = m.width - 30
	}
	rightW := m.width - leftW
	left := zone.Mark(listZone, listPane(leftW, topH))
	detail := m.renderDetailColumn(rightW, topH, m.focus == paneDetail)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, detail)
}

// renderDetailColumn renders the right-hand detail surface, splitting off a
// scrollable app-logs pane beneath it when the Overview tab is showing a site
// with declared log files. Falls back to the plain detail pane otherwise.
func (m *Model) renderDetailColumn(w, h int, focused bool) string {
	// A bottom logs sub-pane opens for the Sites Overview (app-log file tail)
	// and for the selected service/worker on the Services tab (streaming logs).
	_, logPath, siteLogs := m.overviewLogsActive()
	svcLogs := m.serviceLogsActive()
	if !siteLogs && !svcLogs {
		return zone.Mark("pane:detail", m.renderDetailInline(w, h, focused))
	}
	// The logs sub-pane takes at least half the detail column so it's actually
	// usable; the detail keeps the rest.
	logsH := h / 2
	if logsH < 6 {
		logsH = 6
	}
	if logsH > h-6 {
		logsH = h - 6
	}
	if logsH < 4 || h-logsH < 4 {
		// Too short to split usefully; show the detail alone.
		return zone.Mark("pane:detail", m.renderDetailInline(w, h, focused))
	}
	detail := zone.Mark("pane:detail", m.renderDetailInline(w, h-logsH, focused))
	var logPane string
	if siteLogs {
		logPane = zone.Mark("pane:overviewlogs", m.renderOverviewLogs(logPath, w, logsH))
	} else {
		logPane = zone.Mark("pane:logs", m.renderLogs(w, logsH))
	}
	return lipgloss.JoinVertical(lipgloss.Left, detail, logPane)
}

// failingWorkerNames returns kind-site pairs ("queue-acme", "vite-shop")
// for every worker reporting failed across the snapshot. Built-in kinds
// (queue / schedule / horizon / reverb) plus custom framework workers
// plus per-worktree workers all funnel through here so the header pill,
// dashboard hero, and future toast notifier render the same names.
func failingWorkerNames(snap Snapshot) []string {
	fw := failingWorkers(snap)
	names := make([]string, len(fw))
	for i := range fw {
		names[i] = fw[i].name
	}
	return names
}

// failingWorker is one failed worker plus the index of the owning site in
// snap.Sites, so the dashboard can make a failing row click through to that
// site's detail.
type failingWorker struct {
	name    string
	siteIdx int
}

// failingWorkers is the single source the name list and the clickable dashboard
// rows both derive from, so their ordering can't drift apart.
func failingWorkers(snap Snapshot) []failingWorker {
	var out []failingWorker
	for i, s := range snap.Sites {
		add := func(kind, site string, failing bool) {
			if failing {
				out = append(out, failingWorker{kind + "-" + site, i})
			}
		}
		add("queue", s.Name, s.QueueFailing)
		add("schedule", s.Name, s.ScheduleFailing)
		add("horizon", s.Name, s.HorizonFailing)
		add("reverb", s.Name, s.ReverbFailing)
		for _, fw := range s.FrameworkWorkers {
			add(fw.Name, s.Name, fw.Failing)
		}
		for _, wt := range s.Worktrees {
			for _, fw := range wt.FrameworkWorkers {
				add(fw.Name, s.Name+"/"+wt.Branch, fw.Failing)
			}
		}
	}
	return out
}

// joinTruncated joins names with ", " up to max entries; anything beyond
// is collapsed into "+N more". Keeps the header pill from spilling onto a
// second row when many workers are failing at once.
func joinTruncated(names []string, max int) string {
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:max], ", ") + fmt.Sprintf(" +%d more", len(names)-max)
}

// siteHasFailingWorker is the predicate the sites pane uses to tint a
// row's name with the failing colour. Mirrors the gates in
// failingWorkerNames but scoped to a single site so we avoid the cost of
// rebuilding the global list per row.
func siteHasFailingWorker(s siteinfo.EnrichedSite) bool {
	if s.QueueFailing || s.ScheduleFailing || s.HorizonFailing || s.ReverbFailing {
		return true
	}
	for _, fw := range s.FrameworkWorkers {
		if fw.Failing {
			return true
		}
	}
	for _, wt := range s.Worktrees {
		for _, fw := range wt.FrameworkWorkers {
			if fw.Failing {
				return true
			}
		}
	}
	return false
}

// footChip is one footer key-hint. action=true colours the key amber (it
// mutates state); otherwise it's accent-coloured navigation/view.
type footChip struct {
	key    string
	label  string
	action bool
}

func nav(key, label string) footChip { return footChip{key, label, false} }
func act(key, label string) footChip { return footChip{key, label, true} }

// renderFootChips joins coloured key-hints with dim dot separators and clips
// the result to the window width.
func (m *Model) renderFootChips(chips []footChip) string {
	parts := make([]string, len(chips))
	for i, c := range chips {
		keyStyle := footNavKeyStyle
		if c.action {
			keyStyle = footActionKeyStyle
		}
		parts[i] = keyStyle.Render(c.key) + " " + footLabelStyle.Render(c.label)
	}
	sep := footLabelStyle.Render("  ·  ")
	return clipLine("  "+strings.Join(parts, sep), m.width)
}

func (m *Model) renderFooter() string {
	if m.filterActive {
		return helpStyle.Render("  filter: type to match · enter apply · esc clear")
	}

	// Narrow terminals only have room for the essentials; `?` reveals the rest.
	if m.width < narrowWidth {
		return m.renderFootChips([]footChip{
			nav("ctrl+←→", "tabs"), nav("↑↓", "nav"), act("space", "toggle"), nav("?", "help"), act("q", "quit"),
		})
	}

	// Context-aware: each tab shows only the keys that act on it, so the bar
	// reads as a relevant cheat-sheet rather than a wall of every binding.
	var chips []footChip
	switch m.activeTab {
	case tabDashboard:
		chips = []footChip{
			nav("ctrl+←→", "tabs"), nav("↑↓", "nav"), nav("tab", "card"), nav("enter", "open"),
			act("H", "heal"), nav("?", "help"), act("q", "quit"),
		}
	case tabServices:
		chips = []footChip{
			nav("ctrl+←→", "tabs"), nav("↑↓", "nav"), nav("/", "filter"),
			act("s", "start"), act("x", "stop"), act("r", "restart"), act("u", "update"), act("b", "rollback"),
			act("t", "shell"), act("O", "open"), nav("?", "help"), act("q", "quit"),
		}
	default: // tabSites
		chips = []footChip{
			nav("ctrl+←→", "tabs"), nav("tab", "panes"), nav("↑↓", "nav"), act("space", "toggle"), nav("/", "filter"),
			act("s", "start"), act("x", "stop"), act("r", "restart"), nav("l", "logs"), act("t", "shell"),
			nav("S", "settings"), nav("Y", "system"), nav("D", "debug"), nav("?", "help"), act("q", "quit"),
		}
	}
	return m.renderFootChips(chips)
}

func (m *Model) renderStatus() string {
	// Palette is now a modal overlay (modal.go); status bar only ever
	// renders the most recent action result. An in-flight verb (status
	// ends with "…") gets a spinner glyph so the user sees the action
	// is alive even when the underlying CLI takes seconds to respond.
	if m.status == "" {
		return ""
	}
	if !m.statusExpiry.IsZero() && time.Now().After(m.statusExpiry) {
		m.status = ""
		return ""
	}
	if strings.HasSuffix(strings.TrimSpace(m.status), "…") {
		return "  " + renderSpinnerStatus(m.status)
	}
	return helpStyle.Render("  " + m.status)
}

func (m *Model) renderSites(w, h int) string {
	style := paneStyle(m.focus == paneSites)
	innerW, innerH := innerSize(style, w, h)

	sites := m.visibleSites()
	total := len(m.snap.Sites)
	title := fmt.Sprintf("Sites (%d/%d · sort: %s)", len(sites), total, m.siteSort.label())
	lines := []string{padToWidth(clipLine(sectionStyle.Render(title), innerW), innerW)}

	// Filter bar appears as a second header row whenever the user has
	// entered any filter text or is currently typing. Keeps the active
	// filter visible at a glance and distinguishes "empty list because no
	// matches" from "empty list because nothing was linked yet".
	activeFilter := m.focus == paneSites && m.filterActive
	if activeFilter || m.siteFilter != "" {
		lines = append(lines, padToWidth(filterBar(m.siteFilter, activeFilter), innerW))
	}

	availRows := innerH - len(lines)
	if availRows < 1 {
		availRows = 1
	}

	contentW := innerW - 1
	if contentW < 10 {
		contentW = innerW
	}

	// cursorLine maps siteCursor (an index into the sites slice) to the rendered
	// line it lands on, which the grouped layout shifts by its headers.
	var rowData []string
	cursorLine := 0
	switch {
	case total == 0:
		rowData = []string{
			padToWidth(dimStyle.Render("no linked sites yet"), contentW),
			padToWidth("", contentW),
			padToWidth(dimStyle.Render("  cd into a project then run ")+accentStyle.Render("lerd link"), contentW),
			padToWidth(dimStyle.Render("  or open the palette with ")+accentStyle.Render(":")+dimStyle.Render(" and type ")+accentStyle.Render("link"), contentW),
		}
	case len(sites) == 0:
		rowData = []string{
			padToWidth(dimStyle.Render("no sites match filter"), contentW),
			padToWidth(dimStyle.Render("  press ")+accentStyle.Render("esc")+dimStyle.Render(" to clear"), contentW),
		}
	case m.siteSort == siteSortWorkspace:
		rowData, cursorLine = renderGroupedSiteRows(sites, m.snap.Workspaces, m.siteCursor, m.focus == paneSites, contentW)
	default:
		for i, s := range sites {
			row := renderSiteRow(i == m.siteCursor && m.focus == paneSites, s, contentW)
			row = padToWidth(clipLine(row, contentW), contentW)
			// Mark the padded row so a click anywhere on it selects this site.
			// The marker wraps the final content, so width math above is
			// unaffected (bubblezone markers are zero-width to ansi).
			rowData = append(rowData, zone.Mark(fmt.Sprintf("site:%d", i), row))
		}
		cursorLine = m.siteCursor
	}

	cur := -1
	if m.focus == paneSites && m.followCursor {
		cur = cursorLine
	}
	visible := viewport(rowData, cur, availRows, &m.siteScroll)
	bar := renderScrollbar(availRows, len(rowData), m.siteScroll, len(visible))
	for i := 0; i < availRows; i++ {
		row := ""
		if i < len(visible) {
			row = visible[i]
		}
		lines = append(lines, padToWidth(row, contentW)+bar[i])
	}
	for len(lines) < innerH {
		lines = append(lines, spaces(innerW))
	}

	return style.Render(strings.Join(lines, "\n"))
}

// siteWorkerColWidth is the fixed display-width reservation for the worker
// glyphs column in the sites list. Needs to be consistent across rows for
// the PHP column (which sits to the left of it) to align cleanly.
// 12 cells fits the typical worst case: q·s·v·h·m·m (6 glyphs + 5 spaces).
const siteWorkerColWidth = 12

func renderSiteRow(selected bool, s siteinfo.EnrichedSite, paneW int) string {
	glyph := fpmGlyph(s)
	workers := workerGlyphs(s)

	// Display the primary domain (the URL users actually visit) rather than
	// the internal site registry name — the name is still used for command
	// dispatch and filtering, it just isn't what shows up in the list.
	name := s.PrimaryDomain()
	if name == "" {
		name = s.Name
	}
	// Group secondaries are listed directly under their main; the marker reads
	// them as a child occupying a subdomain of the main above.
	if s.GroupSubdomain != "" {
		name = "↳ " + name
	}
	if s.Paused {
		name += " (paused)"
	}

	// Reserve the SAME budget on every row so the worker column lines up
	// vertically, regardless of which workers a site happens to run. The
	// previous version subtracted workersW per row, which left empty-worker
	// rows with a wider name column.
	reserved := 4 /* prefix + glyph + spaces */ + 1 + siteWorkerColWidth
	nameW := paneW - reserved
	if nameW < 16 {
		nameW = 16
	}
	name = padRight(truncatePlain(name, nameW), nameW)

	prefix := " "
	if selected {
		prefix = accentStyle.Render("▸")
	}

	styled := name
	switch {
	case s.Paused:
		styled = pausedStyle.Render(name)
	case siteHasFailingWorker(s):
		// Tint the whole row so a healthy-looking site with one bad
		// worker isn't mistaken for a fully-green one at a glance.
		// Selected sites keep the accent treatment; failing colour wins
		// only when the user isn't already pointing at this row.
		if selected {
			styled = selectedStyle.Render(name)
		} else {
			styled = failingStyle.Render(name)
		}
	case selected:
		styled = selectedStyle.Render(name)
	}

	return fmt.Sprintf("%s %s %s %s", prefix, glyph, styled, padToWidth(workers, siteWorkerColWidth))
}

func fpmGlyph(s siteinfo.EnrichedSite) string {
	if s.Paused {
		return pausedStyle.Render(glyphPaused)
	}
	if s.FPMRunning {
		return runningStyle.Render(glyphRunning)
	}
	return stoppedStyle.Render(glyphStopped)
}

func workerGlyphs(s siteinfo.EnrichedSite) string {
	var out []string
	add := func(has, running, failing, suspended bool, label string) {
		if !has {
			return
		}
		st, _, _ := workerVisual(failing, running, suspended)
		out = append(out, st.Render(label))
	}
	add(s.HasQueueWorker, s.QueueRunning, s.QueueFailing, workerSuspended(&s, "queue"), "q")
	add(s.HasScheduleWorker, s.ScheduleRunning, s.ScheduleFailing, workerSuspended(&s, "schedule"), "s")
	add(s.HasReverb, s.ReverbRunning, s.ReverbFailing, workerSuspended(&s, "reverb"), "v")
	add(s.HasHorizon, s.HorizonRunning, s.HorizonFailing, workerSuspended(&s, "horizon"), "h")
	for _, fw := range s.FrameworkWorkers {
		add(true, fw.Running, fw.Failing, workerSuspended(&s, fw.Name), "•")
	}
	return strings.Join(out, " ")
}

func (m *Model) renderServices(w, h int) string {
	style := paneStyle(m.focus == paneServices)
	innerW, innerH := innerSize(style, w, h)

	services := m.visibleServices()
	total := len(m.snap.Services)
	title := fmt.Sprintf("Services (%d/%d · sort: %s)", len(services), total, m.svcSort.label())
	lines := []string{padToWidth(clipLine(sectionStyle.Render(title), innerW), innerW)}

	activeFilter := m.focus == paneServices && m.filterActive
	if activeFilter || m.svcFilter != "" {
		lines = append(lines, padToWidth(filterBar(m.svcFilter, activeFilter), innerW))
	}

	availRows := innerH - len(lines)
	if availRows < 1 {
		availRows = 1
	}

	contentW := innerW - 1
	if contentW < 10 {
		contentW = innerW
	}

	var rowData []string
	// cursorLine maps svcCursor (an index into the services slice) to the
	// row position in rowData, accounting for non-focusable group headers.
	// Defaults to 0; if grouped rendering inserts headers, this is updated
	// per service-row so viewport keeps the selection on screen.
	cursorLine := 0
	switch {
	case total == 0:
		rowData = []string{
			padToWidth(dimStyle.Render("no services configured"), contentW),
			padToWidth("", contentW),
			padToWidth(dimStyle.Render("  link a site or install a preset (e.g. ")+accentStyle.Render("lerd preset install mysql")+dimStyle.Render(")"), contentW),
		}
	case len(services) == 0:
		rowData = []string{
			padToWidth(dimStyle.Render("no services match filter"), contentW),
			padToWidth(dimStyle.Render("  press ")+accentStyle.Render("esc")+dimStyle.Render(" to clear"), contentW),
		}
	default:
		rowData, cursorLine = renderGroupedServiceRows(services, m.svcCursor, m.focus == paneServices, contentW)
	}

	cur := -1
	if m.focus == paneServices && m.followCursor {
		cur = cursorLine
	}
	visible := viewport(rowData, cur, availRows, &m.svcScroll)
	bar := renderScrollbar(availRows, len(rowData), m.svcScroll, len(visible))
	for i := 0; i < availRows; i++ {
		row := ""
		if i < len(visible) {
			row = visible[i]
		}
		lines = append(lines, padToWidth(row, contentW)+bar[i])
	}
	for len(lines) < innerH {
		lines = append(lines, spaces(innerW))
	}

	return style.Render(strings.Join(lines, "\n"))
}

// filterBar renders the single-line filter chrome shown above the list:
// "filter: <text>▌" while typing, "filter: <text>" otherwise. Kept
// unstyled-plain so the caller can pad it reliably to the pane width.
func filterBar(text string, active bool) string {
	label := dimStyle.Render("  filter: ")
	if active {
		return label + text + "▌"
	}
	if text == "" {
		return ""
	}
	return label + text
}

// serviceGroup labels the bucket a ServiceRow lands in for the grouped
// services pane. Order here drives the visual order: Core first (the
// long-lived presets), then Custom (user-installed), then Workers (the
// per-site fan-out at the bottom because it can be long).
type serviceGroup int

const (
	groupCore serviceGroup = iota
	groupCustom
	groupWorkers
)

func (g serviceGroup) label() string {
	switch g {
	case groupCustom:
		return "Custom"
	case groupWorkers:
		return "Workers"
	}
	return "Core"
}

// classifyService returns the group a row belongs to. Workers carry a
// WorkerKind tag; custom services have Custom=true; everything else is
// a default preset (Core).
func classifyService(s ServiceRow) serviceGroup {
	switch {
	case s.WorkerKind != "":
		return groupWorkers
	case s.Custom:
		return groupCustom
	default:
		return groupCore
	}
}

// renderGroupedSiteRows interleaves a header per workspace into the site-row
// stream and reports the line index of the focused site. The cursor still
// indexes the flat sites slice, so navigation never lands on a header.
func renderGroupedSiteRows(sites []siteinfo.EnrichedSite, workspaces []config.Workspace, cursor int, paneFocused bool, contentW int) (rows []string, cursorLine int) {
	of := siteWorkspaces(sites, workspaces)
	rows = make([]string, 0, len(sites)+len(workspaces)*2)
	current := ""
	first := true
	for i, s := range sites {
		if ws := of[s.Name]; ws != current || first {
			if ws != "" {
				if !first {
					rows = append(rows, padToWidth("", contentW))
				}
				rows = append(rows, padToWidth("  "+sectionStyle.Render(ws), contentW))
			} else if !first {
				rows = append(rows, padToWidth("", contentW))
			}
			current = ws
			first = false
		}
		if i == cursor && paneFocused {
			cursorLine = len(rows)
		}
		row := renderSiteRow(i == cursor && paneFocused, s, contentW)
		row = padToWidth(clipLine(row, contentW), contentW)
		rows = append(rows, zone.Mark(fmt.Sprintf("site:%d", i), row))
	}
	return rows, cursorLine
}

// renderGroupedServiceRows interleaves dim section headers (Core / Custom
// / Workers) into the service-row stream and reports the line index of
// the focused service so the viewport keeps it visible. Cursor still
// indexes the flat services slice unchanged — only the visual layout is
// grouped, navigation never lands on a header.
func renderGroupedServiceRows(services []ServiceRow, cursor int, paneFocused bool, contentW int) (rows []string, cursorLine int) {
	rows = make([]string, 0, len(services)+6)
	currentGroup := serviceGroup(-1)
	currentSite := ""
	for i, s := range services {
		g := classifyService(s)
		if g != currentGroup {
			if currentGroup != -1 {
				rows = append(rows, padToWidth("", contentW))
			}
			rows = append(rows, padToWidth("  "+sectionStyle.Render(g.label()), contentW))
			currentGroup = g
			currentSite = ""
		}
		// Within Workers, sub-group by owning site: the site shows once as a
		// dim header and each worker below it reads as just its kind + state.
		if g == groupWorkers && s.WorkerSite != currentSite {
			rows = append(rows, padToWidth("    "+dimStyle.Render(s.WorkerSite), contentW))
			currentSite = s.WorkerSite
		}
		if i == cursor && paneFocused {
			cursorLine = len(rows)
		}
		selected := i == cursor && paneFocused
		var row string
		if g == groupWorkers {
			row = renderWorkerRow(selected, s, contentW)
		} else {
			row = renderServiceRow(selected, s, contentW)
		}
		row = padToWidth(clipLine(row, contentW), contentW)
		rows = append(rows, zone.Mark(fmt.Sprintf("svc:%d", i), row))
	}
	return rows, cursorLine
}

// serviceMetaColWidth is the fixed budget for the trailing meta column
// (version + site count + pinned/custom tags). Reserved identically on
// every row so the meta column starts at the same column regardless of
// which tags are present, mirroring the aligned layout in the sites pane.
const serviceMetaColWidth = 18

// serviceStateGlyph maps a service/worker state to its coloured dot, shared by
// the service and worker row renderers so the two never drift.
func serviceStateGlyph(state ServiceState) string {
	switch state {
	case stateRunning:
		return runningStyle.Render(glyphRunning)
	case statePaused:
		return pausedStyle.Render(glyphPaused)
	case stateSuspended:
		return suspendedStyle.Render(glyphSuspended)
	default:
		return stoppedStyle.Render(glyphStopped)
	}
}

// workerKindColWidth aligns the state column across worker rows. 14 cells fit
// the longest framework worker kind (e.g. "broadcaster") without truncating.
const workerKindColWidth = 14

// renderWorkerRow draws a worker beneath its site sub-header in the Workers
// group: just the kind and a state word, since the owning site is already the
// header above it. Indented one level deeper than the site header so the
// nesting reads at a glance.
func renderWorkerRow(selected bool, s ServiceRow, paneW int) string {
	prefix := "   "
	if selected {
		prefix = "  " + accentStyle.Render("▸")
	}
	kind := padRight(truncatePlain(s.WorkerKind, workerKindColWidth), workerKindColWidth)
	if selected {
		kind = selectedStyle.Render(kind)
	}
	return fmt.Sprintf("%s %s %s %s", prefix, serviceStateGlyph(s.State), kind, serviceStateText(s.State))
}

func renderServiceRow(selected bool, s ServiceRow, paneW int) string {
	glyph := serviceStateGlyph(s.State)

	// The site-count lives in the service detail pane now, so the list row
	// just carries the version and any pinned/custom tags.
	meta := ""
	if s.Version != "" {
		meta = dimStyle.Render(s.Version)
	}
	if s.Pinned {
		meta += "  " + accentStyle.Render("pinned")
	}
	if s.Custom {
		meta += "  " + dimStyle.Render("custom")
	}

	reserved := 5 /* two prefix spaces + glyph + spaces */ + serviceMetaColWidth + 1
	nameW := paneW - reserved
	if nameW < 14 {
		nameW = 14
	}
	name := padRight(truncatePlain(s.Name, nameW), nameW)
	styledName := name
	if selected {
		styledName = selectedStyle.Render(name)
	}

	prefix := " "
	if selected {
		prefix = accentStyle.Render("▸")
	}
	return fmt.Sprintf(" %s %s %s %s", prefix, glyph, styledName, padToWidth(meta, serviceMetaColWidth))
}

func (m *Model) renderLogs(w, h int) string {
	style := unfocusedPane
	innerW, innerH := innerSize(style, w, h)

	target := m.logTail.Target()
	label := target.Label
	if label == "" {
		label = target.ID
	}
	title := fmt.Sprintf("Logs · %s", label)
	if n := len(m.currentLogTargets()); n > 1 {
		title += fmt.Sprintf("   [%d/%d · [ ] to switch]", m.logCursor+1, n)
	}
	if m.logScroll > 0 {
		title += dimStyle.Render(fmt.Sprintf("   ↑%d  } to tail", m.logScroll))
	}
	if m.logFilter != "" {
		title += "   " + accentStyle.Render("filter: ")
		title += m.logFilter
	}

	availRows := innerH - 1
	if availRows < 1 {
		availRows = 1
	}
	// Filter input bar steals one row when active so the user sees what
	// they're typing without losing the log header.
	if m.logFilterActive {
		availRows--
		if availRows < 1 {
			availRows = 1
		}
	}

	all := m.logTail.Lines()
	total := len(all)

	// Reserve the rightmost column for the scrollbar. Log lines go in
	// contentW; scrollbar gets 1 cell. lipgloss.Width() is skipped here
	// because it treats horizontal padding as part of the width budget,
	// which makes our already-innerW-wide lines wrap to an extra row.
	contentW := innerW - 1
	if contentW < 10 {
		contentW = innerW
	}

	// Clamp logScroll so it can't scroll past the beginning.
	if m.logScroll > total-availRows {
		m.logScroll = max(0, total-availRows)
	}

	var visible []string
	start := 0
	if total > 0 {
		end := total - m.logScroll
		if end < availRows {
			end = availRows
		}
		if end > total {
			end = total
		}
		start = end - availRows
		if start < 0 {
			start = 0
		}
		visible = all[start:end]
	}

	body := make([]string, 0, availRows)
	for _, ln := range visible {
		body = append(body, clipLine(styleLogLine(ln, m.logFilter), contentW))
	}
	if total == 0 {
		body = append(body, clipLine(dimStyle.Render("waiting for output…"), contentW))
	}
	for len(body) < availRows {
		body = append(body, "")
	}

	bar := renderScrollbar(availRows, total, start, len(visible))

	lines := make([]string, 0, availRows+2)
	lines = append(lines, padToWidth(clipLine(sectionStyle.Render(title), innerW), innerW))
	if m.logFilterActive {
		lines = append(lines, padToWidth(filterBar(m.logFilter, true), innerW))
	}
	for i := 0; i < availRows; i++ {
		lines = append(lines, padToWidth(body[i], contentW)+bar[i])
	}

	return style.Render(strings.Join(lines, "\n"))
}

// padToWidth right-pads an ANSI-aware string with spaces to `w` display
// cells. Used instead of Go's `%-*s` (which counts bytes) or
// lipgloss.Style.Width (which treats padding as part of the block width
// and causes lines to wrap when we want them to sit flush with the border).
func padToWidth(s string, w int) string {
	n := ansi.StringWidth(s)
	if n >= w {
		return s
	}
	return s + spaces(w-n)
}

// clipLine truncates s to display width w without slicing through an ANSI
// escape or a multi-byte rune. Uses ansi.Truncate so styled log output is
// preserved even when the line is too wide for the pane.
func clipLine(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// renderScrollbar returns a slice of height strings (one per content row)
// drawing a vertical scrollbar for a virtual list of `total` items where
// `visible` items starting at `start` are on-screen. Each entry is a
// single-cell string so the caller appends it to the rightmost column.
func renderScrollbar(height, total, start, visible int) []string {
	out := make([]string, height)
	if height <= 0 {
		return out
	}
	if total <= visible || total == 0 {
		// Nothing to scroll: leave the column blank rather than drawing a full
		// track, which otherwise reads as a stray second border inside the box.
		for i := range out {
			out[i] = " "
		}
		return out
	}
	thumbSize := height * visible / total
	if thumbSize < 1 {
		thumbSize = 1
	}
	if thumbSize > height {
		thumbSize = height
	}
	// Track position proportionally: thumbStart ∈ [0, height-thumbSize].
	maxStart := total - visible
	thumbStart := 0
	if maxStart > 0 {
		thumbStart = start * (height - thumbSize) / maxStart
	}
	for i := 0; i < height; i++ {
		if i >= thumbStart && i < thumbStart+thumbSize {
			out[i] = accentStyle.Render("█")
		} else {
			out[i] = dimStyle.Render("│")
		}
	}
	return out
}

func paneStyle(focused bool) lipgloss.Style {
	if focused {
		return focusedPane
	}
	return unfocusedPane
}

func innerSize(style lipgloss.Style, w, h int) (int, int) {
	hf := style.GetHorizontalFrameSize()
	vf := style.GetVerticalFrameSize()
	return max(1, w-hf), max(1, h-vf)
}

// viewport returns the slice of rows that fit in `height`, scrolled so the
// cursor stays visible. scroll is updated in place so the pane remembers
// where it was between frames. Pass cursor < 0 for pure scroll surfaces
// (no selection); viewport then leaves scroll alone except for clamping
// against the content bounds, so the user's manual scroll position sticks.
func viewport(rows []string, cursor, height int, scroll *int) []string {
	if height <= 0 || len(rows) == 0 {
		return nil
	}
	if cursor >= 0 {
		if cursor < *scroll {
			*scroll = cursor
		}
		if cursor >= *scroll+height {
			*scroll = cursor - height + 1
		}
	}
	if *scroll < 0 {
		*scroll = 0
	}
	if maxScroll := len(rows) - height; *scroll > maxScroll {
		if maxScroll < 0 {
			maxScroll = 0
		}
		*scroll = maxScroll
	}
	end := *scroll + height
	if end > len(rows) {
		end = len(rows)
	}
	return rows[*scroll:end]
}

func truncate(s string, w int) string {
	if w <= 0 || len(s) <= w {
		return s
	}
	if w <= 1 {
		return "…"
	}
	return s[:w-1] + "…"
}

// truncatePlain truncates a rune string by display length without slicing
// through a multi-byte rune. Only safe on unstyled text; never pass ANSI-
// wrapped strings here since escape bytes would count against the budget.
func truncatePlain(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

// padRight right-pads s with spaces to display width w (rune-counted, unstyled).
func padRight(s string, w int) string {
	n := len([]rune(s))
	if n >= w {
		return s
	}
	return s + spaces(w-n)
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
