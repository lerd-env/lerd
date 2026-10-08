package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

// coreProcess is one of lerd's own processes the sidebar footer lists: what it
// does, where its logs come from, and how the status snapshot reports it.
type coreProcess struct {
	name  string
	about string
	logs  LogTarget
	state func(StatusRow) (ok bool, word string)
}

var coreProcesses = []coreProcess{
	{
		name:  "dns",
		about: "resolves the site domains to this machine",
		logs:  LogTarget{Kind: kindJournal, ID: "lerd-dns", Label: "dns"},
		state: func(s StatusRow) (bool, string) {
			switch {
			case s.DNSDisabled:
				return true, "off"
			case s.DNSOk:
				return true, "resolving"
			case s.DNSDegraded:
				return false, "degraded"
			}
			return false, "down"
		},
	},
	{
		name:  "nginx",
		about: "serves every site and proxies to PHP and the workers",
		logs:  LogTarget{Kind: kindPodman, ID: "lerd-nginx", Label: "nginx"},
		state: func(s StatusRow) (bool, string) { return runningWord(s.NginxRunning) },
	},
	{
		name:  "watcher",
		about: "keeps workers, vhosts and config in step with your projects",
		logs:  LogTarget{Kind: kindJournal, ID: "lerd-watcher", Label: "watcher"},
		state: func(s StatusRow) (bool, string) { return runningWord(s.WatcherRunning) },
	},
}

func runningWord(ok bool) (bool, string) {
	if ok {
		return true, "running"
	}
	return false, "stopped"
}

func coreByName(name string) (coreProcess, bool) {
	for _, c := range coreProcesses {
		if c.name == name {
			return c, true
		}
	}
	return coreProcess{}, false
}

// renderCoreView is a core process's state over its live log tail.
func (m *Model) renderCoreView(w, h int) string {
	c, _ := coreByName(m.coreName)
	cw := m.contentWidth(w)
	ok, word := c.state(m.snap.Status)
	glyph, fg := sp(glyphRunning+" ", colRunning), colDim
	if !ok {
		glyph, fg = bd(glyphFailing+" ", colFailing), colFailing
	}
	head := []string{
		row(nil, cw), crumbRow(cw, []string{"Core", c.name}, nil), row(nil, cw),
		row(nil, cw, bd(c.name, nil), sp("   "+c.about, colDim)),
		row(nil, cw, glyph, sp(word, fg)),
		row(nil, cw),
	}
	body := zone.Mark("pane:logs", m.renderLogsIn(bareFrame, cw, max(3, h-len(head)), nil))
	return frameLines(w, h, cw, append(head, strings.Split(body, "\n")...))
}

// handleCoreKey gives a core view its one action: bring lerd's processes up.
func (m *Model) handleCoreKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.activeTab != tabCore || msg.String() != "s" {
		return nil, false
	}
	m.setStatus("starting lerd…", 10*time.Second)
	return tea.Sequence(runLerd("", "start"), loadCmd()), true
}
