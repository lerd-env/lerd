package tui

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/geodro/lerd/internal/stats"
	zone "github.com/lrstanley/bubblezone/v2"
)

// statsPollInterval is how often the background poller refreshes resource
// stats. Matches lerd-ui's handleStats cache TTL so the two surfaces show
// the same data at the same cadence; a faster TUI poll would just churn
// `podman stats` without users noticing.
const statsPollInterval = 3 * time.Second

// statsMsg delivers a fresh stats snapshot from the background goroutine to
// the bubbletea program. The poller is started by Run alongside the dumps
// listener so the dashboard pane always has data to render.
type statsMsg struct{ snap stats.Snapshot }

// runStatsPoller fetches a snapshot via stats.Cached on every tick and
// forwards it to the program. Going through Cached (rather than Read
// directly) means the TUI shares lerd-ui's cached snapshot when both are
// running, halving the `podman stats` invocations for the typical
// "dashboard open + TUI open" workflow. Cancelled by ctx so the loop
// exits cleanly on quit.
func runStatsPoller(ctx context.Context, p *tea.Program) {
	ticker := time.NewTicker(statsPollInterval)
	defer ticker.Stop()
	// First read happens immediately so the user doesn't see "no stats"
	// for a full tick after entering the dashboard.
	p.Send(statsMsg{snap: stats.Cached(statsPollInterval)})
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.Send(statsMsg{snap: stats.Cached(statsPollInterval)})
		}
	}
}

// numDashCards is the number of cards in the dashboard grid; it bounds the
// per-card focus index and scroll-offset array on the model.
const numDashCards = 6

// cardContent is a dashboard card's full body. rowZones maps a body line index
// to a bubblezone id so individual rows (sites, services) become clickable —
// the rest of the lines are plain.
type cardContent struct {
	lines    []string
	rowZones map[int]string
}

// orderedZoneIDs returns a card's clickable zone ids ordered by line, so the
// row cursor index lines up with the visual top-to-bottom order of the rows.
func orderedZoneIDs(c cardContent) []string {
	if len(c.rowZones) == 0 {
		return nil
	}
	lis := make([]int, 0, len(c.rowZones))
	for li := range c.rowZones {
		lis = append(lis, li)
	}
	sort.Ints(lis)
	ids := make([]string, len(lis))
	for i, li := range lis {
		ids[i] = c.rowZones[li]
	}
	return ids
}

// dashSelGutter is the 2-col left margin on a selectable dashboard row: a caret
// in the accent (navigation) colour when selected, blank otherwise. Reserving
// it on every selectable row keeps the rows aligned whether or not focused.
func dashSelGutter(rowIdx, sel int) string {
	if rowIdx == sel {
		return accentStyle.Render("▸ ")
	}
	return "  "
}

// renderDashboardGrid draws the Dashboard tab as a responsive grid of six
// cards mirroring the lerd web UI: Sites, Services, Workers, System Health,
// Resources and Lerd. Three columns when wide, two at medium width, one when
// narrow. Each card shows its whole list and scrolls within its own box; the
// focused card (j/k or mouse wheel) gets an accent border.
func (m *Model) renderDashboardGrid(w, h int) string {
	cols := 3
	switch {
	case w < 70:
		cols = 1
	case w < narrowWidth:
		cols = 2
	}
	// No gap between cards: tmux-style flush panels read cleaner than columns
	// floating apart, especially in terminals that don't draw inter-pane gaps.
	const gap = 0
	cardW := (w - (cols-1)*gap) / cols
	if cardW < 24 {
		cardW = 24
	}
	innerW := cardW - 4 // rounded border (2) + horizontal padding (2)
	if innerW < 16 {
		innerW = 16
	}

	titles := []string{"Sites", "Services", "Workers", "System Health", "Resources", "Lerd"}
	cw := innerW - 2 // content width inside the 1-col gap + scrollbar column

	// The focused navigable card gets its selected-row index so it can draw the
	// caret; every other card passes -1 (no selection highlight).
	sels := [numDashCards]int{-1, -1, -1, -1, -1, -1}
	if m.activeTab == tabDashboard {
		sels[m.dashFocus] = m.dashRowCursor[m.dashFocus]
	}
	contents := []cardContent{
		m.dashSitesCard(cw, sels[0]),
		m.dashServicesCard(cw, sels[1]),
		m.dashWorkersCard(cw, sels[2]),
		m.dashSystemHealthCard(cw),
		m.dashResourcesCard(cw),
		m.dashLerdCard(cw),
	}
	// Cache each card's ordered clickable zone ids so the row cursor and enter
	// resolve against exactly what was just rendered.
	for i := range contents {
		m.dashZones[i] = orderedZoneIDs(contents[i])
	}

	rows := (numDashCards + cols - 1) / cols
	cardH := h / rows
	// Floor at 3 (border + border + a line) only when it still fits, so a short
	// terminal shrinks every card rather than clipping whole cards off the
	// bottom via the safety net below.
	if cardH < 3 && rows*3 <= h {
		cardH = 3
	}
	if cardH < 1 {
		cardH = 1
	}
	boxes := make([]string, numDashCards)
	for i := range contents {
		boxes[i] = m.renderScrollableCard(i, titles[i], contents[i], innerW, cardH)
	}

	spacer := strings.Repeat(" ", gap)
	var rowStrs []string
	for r := 0; r < rows; r++ {
		var rowParts []string
		for c := 0; c < cols; c++ {
			idx := r*cols + c
			if idx >= len(boxes) {
				break
			}
			if c > 0 {
				rowParts = append(rowParts, spacer)
			}
			rowParts = append(rowParts, boxes[idx])
		}
		rowStrs = append(rowStrs, lipgloss.JoinHorizontal(lipgloss.Top, rowParts...))
	}
	grid := lipgloss.JoinVertical(lipgloss.Left, rowStrs...)
	// Safety net: never let the grid push the footer off-screen if rounding
	// made it a hair taller than the body budget.
	if gl := strings.Split(grid, "\n"); len(gl) > h {
		grid = strings.Join(gl[:h], "\n")
	}
	return grid
}

// renderScrollableCard boxes a titled card and shows a scroll window over its
// full content. The whole card is a bubblezone ("card:<idx>") for wheel/click
// hit-testing; clickable rows carry their own marks. The focused card gets an
// accent border.
func (m *Model) renderScrollableCard(idx int, title string, c cardContent, innerW, cardH int) string {
	bodyH := cardH - 2 // top + bottom border
	if bodyH < 1 {
		bodyH = 1
	}
	avail := bodyH - 1 // title row
	if avail < 1 {
		avail = 1
	}

	maxScroll := len(c.lines) - avail
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := m.dashScroll[idx]
	// Keep the selected row in view on the focused navigable card.
	if selLine := m.dashSelectedLine(idx, c); selLine >= 0 {
		if selLine < scroll {
			scroll = selLine
		}
		if selLine >= scroll+avail {
			scroll = selLine - avail + 1
		}
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	m.dashScroll[idx] = scroll

	contentW := innerW - 2 // reserve a 1-col gap before the scrollbar column
	if contentW < 1 {
		contentW = innerW
	}

	visible := len(c.lines) - scroll
	if visible > avail {
		visible = avail
	}
	if visible < 0 {
		visible = 0
	}
	bar := renderScrollbar(avail, len(c.lines), scroll, visible)

	body := []string{padToWidth(clipLine(sectionStyle.Render(title), innerW), innerW)}
	for i := 0; i < avail; i++ {
		row := spaces(contentW)
		if li := scroll + i; li < len(c.lines) {
			row = padToWidth(clipLine(c.lines[li], contentW), contentW)
			if z, ok := c.rowZones[li]; ok {
				row = zone.Mark(z, row)
			}
		}
		body = append(body, row+" "+bar[i])
	}

	style := cardStyle
	if m.activeTab == tabDashboard && m.dashFocus == idx {
		style = style.BorderForeground(colAccent)
	}
	return zone.Mark(fmt.Sprintf("card:%d", idx), style.Render(strings.Join(body, "\n")))
}

// dashSelectedLine returns the line index of the focused navigable card's
// selected row, or -1 when this card isn't the focused one or has no selection.
func (m *Model) dashSelectedLine(idx int, c cardContent) int {
	if m.activeTab != tabDashboard || m.dashFocus != idx {
		return -1
	}
	zones := m.dashZones[idx]
	if len(zones) == 0 {
		return -1
	}
	cur := clamp(m.dashRowCursor[idx], 0, len(zones)-1)
	want := zones[cur]
	for li, id := range c.rowZones {
		if id == want {
			return li
		}
	}
	return -1
}

// dashRowRight lays a label on the left and its value flush against the right
// edge of width, with the gap between them, so columns of values line up at
// the card's right side. Both label and value may already be styled.
func dashRowRight(label, value string, width int) string {
	gap := width - lipgloss.Width(label) - lipgloss.Width(value)
	if gap < 1 {
		gap = 1
	}
	return label + spaces(gap) + value
}

func (m *Model) dashSitesCard(width, sel int) cardContent {
	running, paused := 0, 0
	for _, s := range m.snap.Sites {
		if s.Paused {
			paused++
		} else if s.FPMRunning {
			running++
		}
	}
	lines := []string{
		fmt.Sprintf("%d total · %s · %s", len(m.snap.Sites),
			runningStyle.Render(fmt.Sprintf("%d running", running)),
			pausedStyle.Render(fmt.Sprintf("%d paused", paused))),
		"",
	}
	if len(m.snap.Sites) == 0 {
		return cardContent{append(lines, dimStyle.Render("no linked sites yet")), nil}
	}
	zones := map[int]string{}
	for i, s := range m.snap.Sites {
		name := s.PrimaryDomain()
		if name == "" {
			name = s.Name
		}
		// Domain on the left, framework label flush right.
		right := ""
		if s.FrameworkLabel != "" {
			right = dimStyle.Render(s.FrameworkLabel)
		}
		zones[len(lines)] = fmt.Sprintf("dashsite:%d", i)
		lines = append(lines, dashSelGutter(i, sel)+dashRowRight(fpmGlyph(s)+" "+name, right, width-2))
	}
	return cardContent{lines, zones}
}

func (m *Model) dashServicesCard(width, sel int) cardContent {
	total, running := 0, 0
	for _, s := range m.snap.Services {
		if s.WorkerKind != "" {
			continue
		}
		total++
		if s.State == stateRunning {
			running++
		}
	}
	lines := []string{
		fmt.Sprintf("%d total · %s", total, runningStyle.Render(fmt.Sprintf("%d running", running))),
		"",
	}
	if total == 0 {
		return cardContent{append(lines, dimStyle.Render("no services configured")), nil}
	}
	zones := map[int]string{}
	si := 0
	for i, s := range m.snap.Services {
		if s.WorkerKind != "" {
			continue
		}
		// Name on the left, version flush right.
		right := ""
		if s.Version != "" {
			right = dimStyle.Render(s.Version)
		}
		zones[len(lines)] = fmt.Sprintf("dashsvc:%d", i)
		lines = append(lines, dashSelGutter(si, sel)+dashRowRight(serviceStateGlyph(s.State)+" "+s.Name, right, width-2))
		si++
	}
	return cardContent{lines, zones}
}

func (m *Model) dashWorkersCard(width, sel int) cardContent {
	active, asleep := 0, 0
	// Track each worker's index into m.snap.Services so a clicked row can map
	// back to the matching service-list entry.
	var workerIdx []int
	for i, s := range m.snap.Services {
		if s.WorkerKind == "" {
			continue
		}
		workerIdx = append(workerIdx, i)
		switch s.State {
		case stateRunning:
			active++
		case stateSuspended:
			asleep++
		}
	}
	failing := failingWorkers(m.snap)
	summary := runningStyle.Render(fmt.Sprintf("%d active", active))
	if asleep > 0 {
		summary += " · " + suspendedStyle.Render(fmt.Sprintf("%d asleep", asleep))
	}
	if len(failing) > 0 {
		summary += " · " + failingStyle.Render(fmt.Sprintf("%d failing", len(failing)))
	}
	lines := []string{summary, ""}
	if len(workerIdx) == 0 && len(failing) == 0 {
		return cardContent{append(lines, runningStyle.Render("all workers healthy")), nil}
	}
	zones := map[int]string{}
	si := 0
	for _, idx := range workerIdx {
		wk := m.snap.Services[idx]
		// The status dot already conveys running/stopped/suspended, so the
		// site sits on the left and the worker kind (queue / schedule / vite)
		// is flush right instead of a redundant state word.
		left := serviceStateGlyph(wk.State) + " " + wk.WorkerSite
		right := dimStyle.Render(wk.WorkerKind)
		if wk.WorkerSite == "" {
			left = serviceStateGlyph(wk.State) + " " + wk.WorkerKind
			right = ""
		}
		zones[len(lines)] = fmt.Sprintf("dashworker:%d", idx)
		lines = append(lines, dashSelGutter(si, sel)+dashRowRight(left, right, width-2))
		si++
	}
	if len(failing) > 0 {
		lines = append(lines, "")
		// Key the zone by the failing-worker index, not f.siteIdx: two failing
		// workers on the same site would otherwise share one id and bubblezone
		// keeps a single region per id, leaving the second row a dead click.
		for fi, f := range failing {
			zones[len(lines)] = fmt.Sprintf("dashfailsite:%d", fi)
			lines = append(lines, dashSelGutter(si, sel)+failingStyle.Render("⚠ "+f.name))
			si++
		}
		lines = append(lines, dimStyle.Render("press H to heal"))
	}
	if len(zones) == 0 {
		zones = nil
	}
	return cardContent{lines, zones}
}

func (m *Model) dashSystemHealthCard(width int) cardContent {
	row := func(label, value string) string {
		return dashRowRight(dimStyle.Render(label), value, width)
	}
	lines := []string{
		row("DNS", dnsHealthText(m.snap.Status)),
		row("Nginx", runningOrStoppedColoured(m.snap.Status.NginxRunning)),
		row("Watcher", runningOrStoppedColoured(m.snap.Status.WatcherRunning)),
	}
	if len(m.snap.Status.PHPRunning) > 0 {
		lines = append(lines, row("PHP FPM", runningStyle.Render(strings.Join(m.snap.Status.PHPRunning, ", "))))
	} else {
		lines = append(lines, row("PHP FPM", dimStyle.Render("none running")))
	}
	return cardContent{lines, nil}
}

func (m *Model) dashResourcesCard(width int) cardContent {
	if !m.stats.Available {
		return cardContent{[]string{dimStyle.Render("collecting…")}, nil}
	}
	lines := []string{dashRowRight("CPU", fmt.Sprintf("%.1f%%", m.stats.TotalCPUPercent), width)}
	memText := stats.FormatBytes(m.stats.TotalMemBytes)
	if m.stats.HostMemBytes > 0 {
		memText += " / " + stats.FormatBytes(m.stats.HostMemBytes)
	}
	lines = append(lines, dashRowRight("Memory", memText, width), "")
	for _, c := range m.stats.Containers {
		name := dimStyle.Render(truncatePlain(c.Name, 18))
		value := fmt.Sprintf("%5.1f%%  %s", c.CPUPercent, stats.FormatBytes(c.MemBytes))
		lines = append(lines, dashRowRight(name, value, width))
	}
	return cardContent{lines, nil}
}

func (m *Model) dashLerdCard(width int) cardContent {
	row := func(label, value string) string {
		return dashRowRight(dimStyle.Render(label), value, width)
	}
	lines := []string{row("Version", m.version)}
	if m.updateAvailable != "" {
		lines = append(lines, accentStyle.Render("update: "+m.updateAvailable))
	}
	// Autostart / LAN come from the periodic snapshot, not a syscall per frame.
	lines = append(lines, row("Autostart", onOffWord(m.snap.Status.Autostart)))
	lines = append(lines, row("LAN", onOffWord(m.snap.Status.LANExposed)))
	lines = append(lines, row("Platform", runtime.GOOS+"/"+runtime.GOARCH))

	lines = append(lines, "", sectionStyle.Render("Recent activity"))
	if len(m.activity) == 0 {
		lines = append(lines, dimStyle.Render("no recent activity"))
	} else {
		for _, e := range m.activity {
			lines = append(lines, e.render())
		}
	}
	return cardContent{lines, nil}
}

// dnsHealthText renders the DNS pill the same way the header does so the
// dashboard mirrors what users already learnt at-a-glance from the top bar.
func dnsHealthText(s StatusRow) string {
	switch {
	case s.DNSDisabled:
		return dimStyle.Render("disabled (system resolver only)")
	case s.DNSOk:
		return runningStyle.Render("ok")
	case s.DNSDegraded:
		return accentStyle.Render("degraded (system resolver bypassed)")
	default:
		return failingStyle.Render("down")
	}
}

func runningOrStoppedColoured(running bool) string {
	if running {
		return runningStyle.Render("running")
	}
	return dimStyle.Render("stopped")
}

func onOffWord(on bool) string {
	if on {
		return runningStyle.Render("on")
	}
	return dimStyle.Render("off")
}
