package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

// The service view mirrors the site view: a fixed header with the service's
// facts over an Overview and a Logs tab.

const (
	svcTabOverview = iota
	svcTabLogs
)

var svcTabLabels = []string{"Overview", "Logs"}

func (m *Model) renderServiceView(w, h int) string {
	svc := m.currentService()
	cw := m.contentWidth(w)
	if svc == nil {
		body := row(nil, cw, sp("No service selected", colDim))
		return m.renderFramed(w, h, []string{"Services"}, nil, body)
	}
	head := m.serviceHeader(svc, cw)
	bodyH := max(3, h-len(head))
	var body string
	if m.svcTab == svcTabLogs {
		body = zone.Mark("pane:logs", m.renderLogsIn(bareFrame, cw, bodyH, nil))
	} else {
		body = zone.Mark("pane:detail", m.renderDetailIn(bareFrame, cw, bodyH, m.focus == paneDetail && !m.sideFocus))
	}
	return frameLines(w, h, cw, append(head, strings.Split(body, "\n")...))
}

func (m *Model) serviceHeader(svc *ServiceRow, cw int) []string {
	out := []string{row(nil, cw), crumbRow(cw, []string{"Services", svc.Name}, nil), row(nil, cw)}

	var version []seg
	if svc.Version != "" {
		version = []seg{sp("version ", colDim), sp(svc.Version, nil)}
	}
	out = append(out, rowLR(nil, cw, []seg{bd(svc.Name, nil), sp("   ", nil)}, version))

	state := []seg{serviceGlyph(svc.State), sp(" "+serviceStateWord(svc.State), colDim)}
	if host, def, extras := servicePortsInfo(svc.Name); host > 0 {
		state = append(state, sp("    localhost:"+strconv.Itoa(host), nil))
		if def > 0 && def != host {
			state = append(state, sp(" (default "+strconv.Itoa(def)+")", colDim))
		}
		for _, e := range extras {
			state = append(state, sp("  + "+e, colDim))
		}
	}
	if svc.Dashboard != "" {
		state = append(state, sp("    "+svc.Dashboard, colAccent))
	}
	var flags []seg
	if svc.Pinned {
		flags = append(flags, sp(glyphRunning+" ", colRunning), sp("pinned, never auto-stops", colDim))
	}
	if svc.Custom {
		if len(flags) > 0 {
			flags = append(flags, sp("     ", nil))
		}
		flags = append(flags, sp("custom", colDim))
	}
	out = append(out, rowLR(nil, cw, append(state, sp("   ", nil)), flags), row(nil, cw))

	var labels strings.Builder
	x, ax, aw := 0, 0, 0
	for i, name := range svcTabLabels {
		if i > 0 {
			labels.WriteString(row(nil, 4))
			x += 4
		}
		label := row(nil, len(name), sp(name, colDim))
		if i == m.svcTab {
			label, ax, aw = row(nil, len(name), bd(name, nil)), x, len(name)
		}
		labels.WriteString(zone.Mark(fmt.Sprintf("svctab:%d", i), label))
		x += len(name)
	}
	line := row(nil, cw, sp(strings.Repeat("─", ax), colDivider), sp(strings.Repeat("━", aw), colAccent), sp(strings.Repeat("─", max(0, cw-ax-aw)), colDivider))
	return append(out, padToWidth(labels.String(), cw), line, row(nil, cw))
}

func serviceStateWord(st ServiceState) string {
	switch st {
	case stateRunning:
		return "running"
	case statePaused:
		return "paused"
	case stateSuspended:
		return "asleep"
	}
	return "stopped"
}

// selectServiceTab switches the service view's tab and points the log tail at
// the service when the Logs tab opens.
func (m *Model) selectServiceTab(tab int) tea.Cmd {
	m.svcTab = tab
	m.detailScroll, m.logScroll = 0, 0
	return m.syncLogs()
}

// handleServiceKey owns the service view's tab keys and its two quick actions.
func (m *Model) handleServiceKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	svc := m.currentService()
	if m.activeTab != tabServices || svc == nil || svc.WorkerKind != "" {
		return nil, false
	}
	switch msg.String() {
	case "1":
		return m.selectServiceTab(svcTabOverview), true
	case "2", "l":
		return m.selectServiceTab(svcTabLogs), true
	case "P":
		verb := "pin"
		if svc.Pinned {
			verb = "unpin"
		}
		m.setStatus(verb+"ning "+svc.Name+"…", 5*time.Second)
		return tea.Sequence(runLerd("", "service", verb, svc.Name), loadCmd()), true
	case "A":
		m.openPaletteIn("", "service preset ")
		return nil, true
	}
	return nil, false
}
