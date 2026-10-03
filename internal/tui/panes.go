package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/geodro/lerd/internal/siteinfo"
	zone "github.com/lrstanley/bubblezone/v2"
)

// narrowWidth is the terminal width below which the TUI switches from the
// two-column layout (sites+services | detail) to a single-column layout
// where only the focused pane fills the full width.
const narrowWidth = 100

// View implements tea.Model.
// View wraps the rendered frame in a tea.View, carrying the alt-screen and
// mouse settings that bubbletea v2 moved off program options and onto the view.
func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *Model) render() string {
	if m.width < 60 || m.height < 12 {
		return "terminal too small (need at least 60×12)\n"
	}

	sideW := layoutFor(m.width, m.height).sideW
	// One column of air on each side keeps the main area off the sidebar's edge.
	mainW := m.width - sideW - 2
	hints := m.renderHints(mainW)
	statusBar := m.renderStatus()

	// Toasts float over the content as an overlay rather than claiming layout
	// rows, so a transient notification never reflows the panes underneath.
	reserved := 1
	if statusBar != "" {
		reserved++
	}
	bodyH := max(6, m.height-reserved)

	// The full-width logs pane is the manual `l` toggle. When the tail already
	// shows inside the detail column (the site Logs tab, or a selected service)
	// the full-width one steps aside rather than drawing the same logs twice.
	showFullLogs := m.showLogs && !m.logsInDetail()

	// Logs pane takes at least half the window when open, and can grow larger,
	// leaving only a sliver of the top pane so the log view dominates.
	logH := 0
	if showFullLogs {
		logH = clamp(max(bodyH/2, m.height/2, 10), 0, bodyH-4)
	}
	topH := max(4, bodyH-logH)

	sections := []string{m.renderBody(mainW, topH)}
	if showFullLogs {
		sections = append(sections, zone.Mark("pane:logs", m.renderLogs(mainW, logH, nil, false)))
	}
	if statusBar != "" {
		sections = append(sections, statusBar)
	}
	sections = append(sections, hints)

	mainLines := strings.Split(lipgloss.JoinVertical(lipgloss.Left, sections...), "\n")
	lines := make([]string, m.height)
	for i := range lines {
		ml := ""
		if i < len(mainLines) {
			ml = mainLines[i]
		}
		lines[i] = paintBackground(" "+padToWidth(clipLine(ml, mainW), mainW)+" ", surf.main)
	}
	switch {
	case sideW > 0:
		side := m.renderSidebar(sideW, m.height)
		for i := range lines {
			lines[i] = side[i] + lines[i]
		}
	case m.sideOverlay:
		side := m.renderSidebar(overlaySideW(m.width), m.height)
		for i := range lines {
			lines[i] = splice(lines[i], side[i], 0)
		}
	}

	switch {
	case m.modalActive():
		lines = overlayCenter(dimScreen(lines), m.renderActiveModal(m.width, m.height), m.width)
	case m.quickActive:
		lines = m.withQuickOverlay(lines)
	}
	out := strings.Join(lines, "\n")
	// Composite toasts over the bottom-right, just above the hint line, without
	// having reserved any rows for them above.
	if stack := m.toastStack(); stack != "" {
		out = overlayBottomRight(out, stack, 1)
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

// renderBody renders the active tab's screen into the given height: the
// six-card dashboard grid, the sites list + site detail, or the services list
// + service detail. Sites/Services reuse the wide/narrow split that the old
// combined layout used, minus the second list pane.
func (m *Model) renderBody(width, topH int) string {
	if m.activeTab == tabDashboard {
		return m.renderDashboard(width, topH)
	}
	if m.activeTab == tabSites {
		return m.renderSitesMain(width, topH)
	}
	if m.activeTab == tabServices {
		return m.renderServiceView(width, topH)
	}
	if m.activeTab == tabCore {
		return m.renderCoreView(width, topH)
	}
	if m.activeTab == tabRuntimes {
		return m.renderRuntimesView(width, topH)
	}

	return m.renderDatabasesView(width, topH)
}

// renderDatabasesView is the databases list beside the selected database's
// detail, borderless under a breadcrumb; the two stack on a narrow pane.
func (m *Model) renderDatabasesView(width, topH int) string {
	cw := m.contentWidth(width)
	bodyH := max(4, topH-3)
	var body []string
	if cw < 90 {
		listH := clamp(bodyH*2/5, 4, max(4, bodyH-4))
		body = append(strings.Split(zone.Mark("pane:databases", m.renderDatabasesIn(bareFrame, cw, listH)), "\n"), row(nil, cw))
		body = append(body, strings.Split(zone.Mark("pane:detail", m.renderDetailIn(bareFrame, cw, bodyH-listH-1, m.focus == paneDetail)), "\n")...)
	} else {
		// Sized to its rows (marker, name, size, scrollbar), so the scrollbar
		// sits against the sizes instead of floating in empty space.
		listW := 2 + dbNameColWidth + 8 + 2
		list := strings.Split(zone.Mark("pane:databases", m.renderDatabasesIn(bareFrame, listW, bodyH)), "\n")
		detail := strings.Split(zone.Mark("pane:detail", m.renderDetailIn(bareFrame, cw-listW-4, bodyH, m.focus == paneDetail)), "\n")
		for i := 0; i < bodyH; i++ {
			l, d := "", ""
			if i < len(list) {
				l = list[i]
			}
			if i < len(detail) {
				d = detail[i]
			}
			body = append(body, padToWidth(l, listW)+row(nil, 4)+padToWidth(d, cw-listW-4))
		}
	}
	count := []seg{sp(fmt.Sprintf("%d databases", len(navigableDBRows(m.dbRows()))), colDim)}
	return m.renderFramed(width, topH, []string{"Databases"}, count, strings.Join(body, "\n"))
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

// footChips are the key hints for whatever has focus, most useful first, so a
// narrow hint line can drop from the end.
func (m *Model) footChips() []footChip {
	if m.sideFocus {
		chips := []footChip{nav("↑↓", "move"), nav("enter", "open"), nav("ctrl+p", "go to or do"), nav("/", "filter"), nav("tab", "main"), nav(":", "commands"), nav("?", "help"), act("q", "quit")}
		if m.sideOverlay {
			chips = append([]footChip{nav("\\", "close")}, chips...)
		}
		return chips
	}
	back := nav("tab", "sidebar")
	switch m.activeTab {
	case tabDashboard:
		return []footChip{back, nav("↑↓", "nav"), nav("enter", "open"), act("H", "heal"), nav("?", "help"), act("q", "quit")}
	case tabServices:
		chips := []footChip{back, nav("1-2", "tabs"), act("s", "start"), act("x", "stop"), act("r", "restart"), act("P", "pin"), act("A", "add preset"),
			act("u", "update"), act("b", "rollback"), act("t", "shell")}
		if svc := m.currentService(); svc != nil && svc.Dashboard != "" {
			chips = append(chips[:6], append([]footChip{act("O", "dashboard")}, chips[6:]...)...)
		}
		return append(chips, nav("?", "help"))
	case tabDatabases:
		return []footChip{nav("↑↓", "nav"), nav("tab", "panes"), act("n", "snapshot"), act("e", "export"), act("c", "create"), act("a", "auto snapshots"), act("K", "keep"), act("R", "refresh"), nav("?", "help")}
	case tabCore:
		return []footChip{back, nav("↑↓", "scroll"), act("s", "start lerd"), nav("?", "help")}
	case tabRuntimes:
		return []footChip{back, nav("↑↓", "move"), act("d", "default"), act("x", "xdebug"), act("i", "install"), act("R", "rebuild"), nav("?", "help")}
	}
	if m.detailMode == detailSettings {
		return []footChip{back, nav("↑↓", "nav"), act("space", "toggle"), nav("esc", "back"), nav("?", "help")}
	}
	if len(timingScopes(m.currentSite())) > 1 {
		return []footChip{back, nav("1-5", "tabs"), nav("b", "worktree"), nav("↑↓", "nav"), act("space", "toggle"), act("s", "start"), act("x", "stop"), act("r", "restart"), nav("l", "logs"),
			act("t", "shell"), nav("?", "help")}
	}
	return []footChip{back, nav("1-5", "tabs"), nav("↑↓", "nav"), act("space", "toggle"), act("s", "start"), act("x", "stop"), act("r", "restart"), nav("l", "logs"),
		act("t", "shell"), nav("S", "settings"), nav("Y", "system"), nav("D", "debug"), nav("?", "help")}
}

// renderHints is the single bottom line of the main area: site health counts
// on the left, key hints on the right, dropping hints that no longer fit.
func (m *Model) renderHints(w int) string {
	if m.filterActive {
		return row(nil, w, sp("  filter  ", colAccent), sp("type to match · enter apply · esc clear", colDim))
	}
	running, paused, failing := 0, 0, 0
	for _, s := range m.snap.Sites {
		switch {
		case siteHasFailingWorker(s):
			failing++
		case s.Paused:
			paused++
		case s.FPMRunning:
			running++
		}
	}
	left := []seg{sp("  ", nil), sp(glyphRunning+" ", colRunning), sp(fmt.Sprintf("%d running", running), colDim)}
	if paused > 0 {
		left = append(left, sp("   "+glyphPaused+" ", colPaused), sp(fmt.Sprintf("%d paused", paused), colDim))
	}
	if failing > 0 {
		left = append(left, bd("   "+glyphFailing+" ", colFailing), sp(fmt.Sprintf("%d failing", failing), colFailing))
	}
	chips := m.footChips()
	for len(chips) > 1 {
		var right []seg
		for i, c := range chips {
			if i > 0 {
				right = append(right, sp("   ", nil))
			}
			keyFg := colAccent
			if c.action {
				keyFg = colPaused
			}
			right = append(right, bd(c.key, keyFg), sp(" "+c.label, colDim))
		}
		right = append(right, sp("  ", nil))
		if segsWidth(left)+segsWidth(right)+2 <= w {
			return rowLR(nil, w, left, right)
		}
		chips = chips[:len(chips)-1]
	}
	return row(nil, w, left...)
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

// renderLogs draws the streaming tail. header is prepended inside the pane (the
// site tab strip, when the tail is the Logs tab rather than the `l` overlay) and
// costs one row each; focused picks the border colour, so the pane reads as part
// of the detail column when it stands in for it.
func (m *Model) renderLogs(w, h int, header []string, focused bool) string {
	return m.renderLogsIn(paneStyle(focused), w, h, header)
}

// renderLogsIn draws the log tail inside style: a bordered pane for the
// full-width overlay and services, a bare frame inside the site view.
func (m *Model) renderLogsIn(style lipgloss.Style, w, h int, header []string) string {
	innerW, innerH := innerSize(style, w, h)

	target := m.logTail.Target()
	label := target.Label
	if label == "" {
		label = target.ID
	}
	// A stopped site with no container and no workers has nothing to tail; say so
	// rather than leaving a dangling "Logs ·" above a permanent "waiting for
	// output…", which reads as a hang.
	targets := m.currentLogTargets()
	noSource := len(targets) == 0
	title := "Logs"
	if !noSource {
		title += " · " + label
	}
	if noSource {
		title += dimStyle.Render("   no log source for this site")
	}
	if n := len(targets); n > 1 {
		title += fmt.Sprintf("   [%d/%d · [ ] to switch]", m.logCursor+1, n)
	}
	if m.logScroll > 0 {
		title += dimStyle.Render(fmt.Sprintf("   ↑%d  } to tail", m.logScroll))
	}
	if m.logFilter != "" {
		title += "   " + accentStyle.Render("filter: ")
		title += m.logFilter
	}

	availRows := innerH - 1 - len(header)
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
	contentW := innerW - 2 // a gap and the scrollbar
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
		empty := "waiting for output…"
		if noSource {
			empty = "nothing to tail: the site has no running container and no workers"
		}
		body = append(body, clipLine(dimStyle.Render(empty), contentW))
	}
	for len(body) < availRows {
		body = append(body, "")
	}

	bar := renderScrollbar(availRows, total, start, len(visible))

	lines := make([]string, 0, availRows+len(header)+2)
	lines = append(lines, header...)
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
			out[i] = "  "
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
		// A thin thumb on a faint track: it says where you are without
		// competing with the content, which a solid accent block did.
		// One blank column keeps the bar off the content beside it.
		if i >= thumbStart && i < thumbStart+thumbSize {
			out[i] = " " + dimStyle.Render("┃")
		} else {
			out[i] = " " + lipgloss.NewStyle().Foreground(colDivider).Render("│")
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
