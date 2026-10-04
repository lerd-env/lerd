package tui

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/geodro/lerd/internal/stats"
	zone "github.com/lrstanley/bubblezone/v2"
)

// The dashboard leads with what needs the user, then the machine's resources
// in one column beside core health and recent activity in the other.

// cpuHistoryLen is how many stats samples the CPU sparkline keeps; at the
// poller's 3s interval that is the last three minutes.
const cpuHistoryLen = 60

// dashAlert is one Needs-attention card. site is empty for a core service.
type dashAlert struct {
	site   string
	title  string
	detail []seg
	worker string // crashed worker kind, restarted by r
}

func (m *Model) dashAlerts() []dashAlert {
	var out []dashAlert
	st := m.snap.Status
	core := func(ok bool, name, why string) {
		if !ok {
			out = append(out, dashAlert{title: name + " is stopped", detail: []seg{sp(why, colDim)}})
		}
	}
	if !st.DNSOk && !st.DNSDisabled {
		why := "." + st.TLD + " domains are not resolving"
		if st.DNSDegraded {
			why = "the system resolver is bypassed, ." + st.TLD + " lookups may fail"
		}
		out = append(out, dashAlert{title: "DNS is down", detail: []seg{sp(why, colDim)}})
	}
	core(st.NginxRunning, "nginx", "sites refuse connections until it runs")
	core(st.WatcherRunning, "The watcher", "workers and config changes are not being tracked")

	for _, s := range m.snap.Sites {
		name := s.PrimaryDomain()
		if name == "" {
			name = s.Name
		}
		crashed := func(kind, where string, failing bool) {
			if failing {
				d := []seg{sp("worker ", colDim), sp(kind, nil), sp(" crashed"+where, colDim)}
				out = append(out, dashAlert{site: s.Name, title: name, detail: d, worker: kind})
			}
		}
		crashed("queue", "", s.QueueFailing)
		crashed("schedule", "", s.ScheduleFailing)
		crashed("horizon", "", s.HorizonFailing)
		crashed("reverb", "", s.ReverbFailing)
		for _, fw := range s.FrameworkWorkers {
			crashed(fw.Name, "", fw.Failing)
		}
		for _, wt := range s.Worktrees {
			for _, fw := range wt.FrameworkWorkers {
				crashed(fw.Name, " on "+wt.Branch, fw.Failing)
			}
		}
	}
	return out
}

// dashOpen is enter on a card: jump to the site that owns the problem.
func (m *Model) dashOpen(a dashAlert) tea.Cmd {
	if a.site == "" {
		return nil
	}
	m.switchTab(tabSites)
	m.selectSiteByName(a.site)
	m.focusMain()
	return m.afterNav()
}

// dashFix is r on a card: the one reversible fix it offers. A crashed worker
// restarts on its own unit; a stopped core service comes back with lerd start.
func (m *Model) dashFix(a dashAlert) tea.Cmd {
	if a.site == "" {
		m.setStatus("starting lerd…", 10*time.Second)
		return tea.Sequence(runLerd("", "start"), loadCmd())
	}
	for i := range m.snap.Services {
		svc := &m.snap.Services[i]
		if svc.WorkerSite == a.site && svc.WorkerKind == a.worker {
			return tea.Sequence(m.workerActionCmd(svc, "restart"), loadCmd())
		}
	}
	// A worktree worker has no row of its own; heal restarts every failed unit.
	return m.actionHealWorkers()
}

// handleDashKey owns the dashboard's keys while it has focus.
func (m *Model) handleDashKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.activeTab != tabDashboard || m.sideFocus {
		return nil, false
	}
	alerts := m.dashAlerts()
	m.dashCursor = clamp(m.dashCursor, 0, max(0, len(alerts)-1))
	page := max(1, m.height/2)
	switch key := msg.String(); key {
	case "pgdown":
		m.dashScroll += page
	case "pgup":
		m.dashScroll = max(0, m.dashScroll-page)
	case "home", "g":
		m.dashScroll = 0
	case "end", "G":
		m.dashScroll = 1 << 20
	case "up", "k", "down", "j":
		// With no cards to pick, the arrows scroll the page instead.
		step := 1
		if key == "up" || key == "k" {
			step = -1
		}
		if len(alerts) == 0 {
			m.dashScroll = max(0, m.dashScroll+step)
		} else {
			m.dashCursor = clamp(m.dashCursor+step, 0, len(alerts)-1)
		}
	case "enter", "space":
		if len(alerts) > 0 {
			return m.dashOpen(alerts[m.dashCursor]), true
		}
	case "r":
		if len(alerts) > 0 {
			return m.dashFix(alerts[m.dashCursor]), true
		}
	default:
		return nil, false
	}
	return nil, true
}

// recordCPU keeps the recent total-CPU samples the sparkline draws.
func (m *Model) recordCPU(s stats.Snapshot) {
	if !s.Available {
		return
	}
	m.cpuHist = append(m.cpuHist, s.TotalCPUPercent)
	if len(m.cpuHist) > cpuHistoryLen {
		m.cpuHist = m.cpuHist[len(m.cpuHist)-cpuHistoryLen:]
	}
}

// renderDashboard draws the dashboard as exactly h lines of width w.
func (m *Model) renderDashboard(w, h int) string {
	lay := layoutFor(m.width, m.height)
	pad := max(1, lay.padX-1)
	cw := w - 2*pad
	blank := row(nil, cw)
	var L []string

	right := []seg{}
	if m.stats.Available {
		right = []seg{sp("cpu ", colDim), sp(fmt.Sprintf("%.2f%%", m.stats.TotalCPUPercent), nil), sp("    mem ", colDim), sp(stats.FormatBytes(m.stats.TotalMemBytes), nil)}
	}
	L = append(L, blank, rowLR(nil, cw, []seg{bd("Dashboard", nil)}, right), blank)

	alerts := m.dashAlerts()
	if len(alerts) == 0 {
		L = append(L, row(nil, cw, sideLabel("Needs attention")), blank)
		L = append(L, m.dashCard(cw, "", false, []seg{sp(glyphRunning+"  ", colRunning), sp("Everything is running", nil)}, nil)...)
	} else {
		hint := []seg{sp("tab ", nil), sp("select", colDim)}
		if !m.sideFocus {
			hint = []seg{sp("esc ", nil), sp("back to sidebar", colDim)}
		}
		L = append(L, rowLR(nil, cw, []seg{sideLabel("Needs attention"), sp(fmt.Sprintf("   %d", len(alerts)), colDim)}, hint), blank)
		m.dashCursor = clamp(m.dashCursor, 0, len(alerts)-1)
		for i, a := range alerts {
			if i > 0 {
				L = append(L, blank)
			}
			L = append(L, m.alertCard(cw, i, a, !m.sideFocus && i == m.dashCursor)...)
		}
	}

	section := func(rows []string) {
		L = append(L, blank)
		L = append(L, rows...)
	}
	if cw >= 90 {
		// Resources gets the first column to itself; System is always short, so
		// Recent stacks under it. The shorter column's last panel stretches so
		// both columns end on the same row.
		gap := 2
		lw, rw := (cw-gap)/2, cw-gap-(cw-gap)/2
		res := m.dashResources(lw - dashPanelPadX*2)
		sys, rec := m.dashSystem(rw-dashPanelPadX*2), m.dashRecent(rw-dashPanelPadX*2)
		left := m.dashPanel("Resources", lw, len(res), res)
		top := append(m.dashPanel("System", rw, len(sys), sys), row(nil, rw))
		right := append(top, m.dashPanel("Recent", rw, len(rec), rec)...)
		if d := len(left) - len(right); d > 0 {
			right = append(top, m.dashPanel("Recent", rw, len(rec)+d, rec)...)
		} else if d < 0 {
			left = m.dashPanel("Resources", lw, len(res)-d, res)
		}
		section(joinColumns(left, lw, right, rw, gap))
	} else {
		res, sys := m.dashResources(cw-dashPanelPadX*2), m.dashSystem(cw-dashPanelPadX*2)
		rec := m.dashRecent(cw - dashPanelPadX*2)
		section(m.dashPanel("System", cw, len(sys), sys))
		section(m.dashPanel("Resources", cw, len(res), res))
		section(m.dashPanel("Recent", cw, len(rec), rec))
	}

	// Everything is kept and the page scrolls, so nothing goes missing on a
	// short terminal; the last line says when more sits below.
	m.dashScroll = clamp(m.dashScroll, 0, max(0, len(L)-h))
	L = L[m.dashScroll:]
	for len(L) < h {
		L = append(L, blank)
	}
	if len(L) > h {
		L = append(L[:h-1], row(nil, cw, sp(fmt.Sprintf("↓ %d more below · pgdn", len(L)-h+1), colDim)))
	}
	edge := row(nil, pad)
	for i := range L {
		L[i] = edge + L[i] + edge
	}
	return zone.Mark("pane:dash", strings.Join(L, "\n"))
}

// dashCard is a tinted block with a padding row above and below, dropped on
// short terminals, and an accent bar down its left edge when selected.
func (m *Model) dashCard(cw int, id string, sel bool, title, detail []seg, hint ...seg) []string {
	bg, bar := surf.s2, sp("  ", nil)
	if sel {
		bg, bar = surf.s3, bd("▌ ", colAccent)
	}
	var out []string
	padRow := func() {
		if !layoutFor(m.width, m.height).compact {
			out = append(out, row(bg, cw, bar))
		}
	}
	padRow()
	out = append(out, rowLR(bg, cw, append([]seg{bar}, title...), append(hint, sp("  ", nil))))
	if detail != nil {
		out = append(out, row(bg, cw, append([]seg{bar, sp("   ", nil)}, detail...)...))
	}
	padRow()
	if id != "" {
		for i := range out {
			out[i] = zone.Mark(id, out[i])
		}
	}
	return out
}

// dashPanelPadX is the tinted margin each side of a panel's content.
const dashPanelPadX = 2

// dashPanel wraps content rows in a tinted block under its title, padded to h
// content rows. Content is drawn dashPanelPadX narrower than w on surf.s2.
func (m *Model) dashPanel(title string, w, h int, content []string) []string {
	edge := row(surf.s2, dashPanelPadX)
	inner := w - dashPanelPadX*2
	var out []string
	padRow := func() {
		if !layoutFor(m.width, m.height).compact {
			out = append(out, row(surf.s2, w))
		}
	}
	padRow()
	out = append(out, edge+row(surf.s2, inner, sideLabel(title))+edge, row(surf.s2, w))
	for i := 0; i < h; i++ {
		r := row(surf.s2, inner)
		if i < len(content) {
			r = content[i]
		}
		out = append(out, edge+r+edge)
	}
	padRow()
	return out
}

func (m *Model) alertCard(cw, i int, a dashAlert, sel bool) []string {
	glyph := bd(glyphFailing+"  ", colFailing)
	var hint []seg
	if sel {
		if a.site != "" {
			hint = append(hint, sp("enter ", nil), sp("open    ", colDim))
		}
		fix := "restart"
		if a.site == "" {
			fix = "start lerd"
		}
		hint = append(hint, sp("r ", nil), sp(fix, colDim))
	}
	return m.dashCard(cw, fmt.Sprintf("dashalert:%d", i), sel, []seg{glyph, bd(a.title, nil)}, a.detail, hint...)
}

func (m *Model) dashResources(w int) []string {
	bg := surf.s2
	if !m.stats.Available {
		return []string{row(bg, w, sp("collecting…", colDim))}
	}
	// Room for the label column and "  1.0GB / 32.0GB" after the bar.
	barW := clamp(w-9-18, 10, 40)
	out := []string{
		row(bg, w, sp(padRight("cpu", 9), colDim), sp(sparkline(m.cpuHist, barW), colRunning), sp(fmt.Sprintf("  %.2f%%", m.stats.TotalCPUPercent), nil)),
		row(bg, w, append([]seg{sp(padRight("memory", 9), colDim)}, memBar(m.stats, barW)...)...),
		row(bg, w),
	}
	top := append([]stats.ContainerStat(nil), m.stats.Containers...)
	sort.SliceStable(top, func(i, j int) bool { return top[i].MemBytes > top[j].MemBytes })
	for _, c := range top {
		name := strings.TrimPrefix(c.Name, "lerd-")
		out = append(out, row(bg, w, sp(padRight("", 9), nil), sp(padRight(truncatePlain(name, 16), 17), nil),
			sp(fmt.Sprintf("%7s", stats.FormatBytes(c.MemBytes)), nil), sp(fmt.Sprintf("   %.2f%%", c.CPUPercent), colDim)))
	}
	return out
}

// sparkline draws the newest w samples, scaled to the window's peak. The scale
// never drops below 5% so one blip on an idle machine does not read as a spike.
func sparkline(samples []float64, w int) string {
	blocks := []rune("▁▂▃▄▅▆▇█")
	if len(samples) > w {
		samples = samples[len(samples)-w:]
	}
	peak := 5.0
	for _, v := range samples {
		peak = max(peak, v)
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", w-len(samples)))
	for _, v := range samples {
		b.WriteRune(blocks[min(len(blocks)-1, int(v/peak*float64(len(blocks)-1)))])
	}
	return b.String()
}

func memBar(s stats.Snapshot, w int) []seg {
	used := stats.FormatBytes(s.TotalMemBytes)
	if s.HostMemBytes <= 0 {
		return []seg{sp(used, nil)}
	}
	fill := clamp(int(float64(w)*float64(s.TotalMemBytes)/float64(s.HostMemBytes)+0.5), 1, w)
	return []seg{sp(strings.Repeat("━", fill), colRunning), sp(strings.Repeat("━", w-fill), colDivider),
		sp("  "+used, nil), sp(" / "+stats.FormatBytes(s.HostMemBytes), colDim)}
}

func (m *Model) dashSystem(w int) []string {
	st := m.snap.Status
	kv := func(k string, v ...seg) []seg {
		return append([]seg{sp(padRight(k, 11), colDim)}, v...)
	}
	state := func(ok bool, on string) []seg {
		if ok {
			return []seg{sp(glyphRunning+" ", colRunning), sp(on, nil)}
		}
		return []seg{bd(glyphFailing+" ", colFailing), sp("stopped", colFailing)}
	}
	dns := state(st.DNSOk, "resolving ."+st.TLD)
	switch {
	case st.DNSDisabled:
		dns = []seg{sp(glyphStopped+" ", colDim), sp("off, system resolver only", colDim)}
	case !st.DNSOk && st.DNSDegraded:
		dns = []seg{sp(glyphPaused+" ", colPaused), sp("degraded", colPaused)}
	}
	running, asleep, crashed := 0, 0, len(failingWorkers(m.snap))
	for _, s := range m.snap.Services {
		switch {
		case s.WorkerKind == "":
		case s.State == stateRunning:
			running++
		case s.State == stateSuspended:
			asleep++
		}
	}
	workers := []seg{sp(fmt.Sprintf("%d running", running), nil)}
	if asleep > 0 {
		workers = append(workers, sp(fmt.Sprintf(" · %d asleep", asleep), colDim))
	}
	if crashed > 0 {
		workers = append(workers, bd("   "+glyphFailing+" ", colFailing), sp(fmt.Sprintf("%d crashed", crashed), colFailing))
	}
	version := []seg{sp(m.version, nil)}
	if m.updateAvailable != "" {
		version = append(version, sp("   update ", colDim), sp(m.updateAvailable, colAccent))
	}
	onOff := func(on bool) seg {
		if on {
			return sp("on", nil)
		}
		return sp("off", colDim)
	}
	facts := [][]seg{
		kv("dns", dns...),
		kv("nginx", state(st.NginxRunning, "running")...),
		kv("watcher", state(st.WatcherRunning, "running")...),
		kv("workers", workers...),
		kv("autostart", onOff(st.Autostart)),
		kv("lan", onOff(st.LANExposed)),
		kv("lerd", version...),
		kv("platform", sp(runtime.GOOS+"/"+runtime.GOARCH, colDim)),
	}
	return systemRows(facts, w)
}

// systemRows pairs the facts into two columns when both halves fit whole, so a
// wide panel is not mostly empty; otherwise it keeps one fact per row.
func systemRows(facts [][]seg, w int) []string {
	const gap = 3
	half := (len(facts) + 1) / 2
	lw, rw := 0, 0
	for i, f := range facts {
		if i < half {
			lw = max(lw, segsWidth(f))
		} else {
			rw = max(rw, segsWidth(f))
		}
	}
	var out []string
	if lw+gap+rw > w {
		for _, f := range facts {
			out = append(out, row(surf.s2, w, f...))
		}
		return out
	}
	for i := 0; i < half; i++ {
		var r []seg
		if half+i < len(facts) {
			r = facts[half+i]
		}
		out = append(out, row(surf.s2, lw+gap, facts[i]...)+row(surf.s2, w-lw-gap, r...))
	}
	return out
}

func (m *Model) dashRecent(w int) []string {
	if len(m.activity) == 0 {
		return []string{row(surf.s2, w, sp("Nothing has changed since the TUI opened", colDim))}
	}
	var out []string
	for _, e := range m.activity {
		dot, text := sp(glyphRunning, colRunning), sp("  "+e.text, nil)
		switch e.tone {
		case toneBad:
			dot, text.fg = bd(glyphFailing, colFailing), colFailing
		case toneWarn:
			dot = sp(glyphPaused, colPaused)
		}
		out = append(out, row(surf.s2, w, sp(padRight(humanAgo(time.Since(e.at)), 7), colDim), dot, text))
	}
	return out
}

// joinColumns sets two blocks side by side, padding the shorter one.
func joinColumns(a []string, aw int, b []string, bw, gap int) []string {
	out := make([]string, max(len(a), len(b)))
	for i := range out {
		l, r := row(nil, aw), row(nil, bw)
		if i < len(a) {
			l = a[i]
		}
		if i < len(b) {
			r = b[i]
		}
		out[i] = l + row(nil, gap) + r
	}
	return out
}
