package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/geodro/lerd/internal/config"
	lerdNode "github.com/geodro/lerd/internal/node"
	phpPkg "github.com/geodro/lerd/internal/php"
	zone "github.com/lrstanley/bubblezone/v2"
)

// The PHP & Node view lists the installed runtimes beside the selected one's
// detail, with the actions that have a CLI verb: set the default, toggle
// Xdebug, rebuild, and install another version through the palette.

type runtimeFacts struct {
	php, node               []string
	defaultPHP, defaultNode string
	xdebug                  map[string]bool
	xdebugMode              map[string]string
	extras                  map[string]string
}

// loadRuntimeFacts reads what is installed and configured. A variable so tests
// can stand in fixed facts instead of the machine's own installs.
var loadRuntimeFacts = func() runtimeFacts {
	f := runtimeFacts{xdebug: map[string]bool{}, xdebugMode: map[string]string{}, extras: map[string]string{}}
	f.php, _ = phpPkg.ListInstalled()
	f.node = lerdNode.ListInstalled()
	cfg, _ := config.LoadGlobal()
	if cfg == nil {
		return f
	}
	f.defaultPHP, f.defaultNode = cfg.PHP.DefaultVersion, cfg.Node.DefaultVersion
	for _, v := range f.php {
		f.xdebug[v] = cfg.IsXdebugEnabled(v)
		f.xdebugMode[v] = cfg.GetXdebugMode(v)
		f.extras[v] = phpExtrasSummary(cfg, v)
	}
	return f
}

type runtimeRow struct{ kind, version string }

func (m *Model) runtimeRows() []runtimeRow {
	f := loadRuntimeFacts()
	var rows []runtimeRow
	for _, v := range f.php {
		rows = append(rows, runtimeRow{"php", v})
	}
	for _, v := range f.node {
		rows = append(rows, runtimeRow{"node", v})
	}
	return rows
}

func (m *Model) currentRuntime() (runtimeRow, bool) {
	rows := m.runtimeRows()
	if len(rows) == 0 {
		return runtimeRow{}, false
	}
	m.rtCursor = clamp(m.rtCursor, 0, len(rows)-1)
	return rows[m.rtCursor], true
}

func (m *Model) renderRuntimesView(w, h int) string {
	cw := m.contentWidth(w)
	f := loadRuntimeFacts()
	rows := m.runtimeRows()
	sel, _ := m.currentRuntime()
	running := map[string]bool{}
	for _, v := range m.snap.Status.PHPRunning {
		running[v] = true
	}

	const listW = 22
	var list []string
	lastKind := ""
	for i, r := range rows {
		if r.kind != lastKind {
			if lastKind != "" {
				list = append(list, row(nil, listW))
			}
			label := "PHP"
			if r.kind == "node" {
				label = "Node"
			}
			list = append(list, row(nil, listW, sideLabel(label)))
			lastKind = r.kind
		}
		glyph := sp(glyphRunning, colRunning)
		if r.kind == "php" && !running[r.version] {
			glyph = sp(glyphStopped, colDim)
		}
		def := f.defaultPHP
		if r.kind == "node" {
			def = f.defaultNode
		}
		var tag []seg
		if r.version == def {
			tag = []seg{sp("default", colDim)}
		}
		marker := sp("  ", nil)
		name := sp(r.version, nil)
		if i == m.rtCursor {
			marker, name = bd("▸ ", colAccent), bd(r.version, nil)
		}
		list = append(list, zone.Mark(fmt.Sprintf("rt:%d", i), rowLR(nil, listW, []seg{marker, glyph, sp("  ", nil), name}, tag)))
	}
	if len(rows) == 0 {
		list = []string{row(nil, listW, sp("nothing installed", colDim))}
	}

	detail := m.runtimeDetail(sel, f, running, cw-listW-4)
	bodyH := max(1, h-3)
	var body []string
	for i := 0; i < bodyH; i++ {
		l, d := row(nil, listW), row(nil, cw-listW-4)
		if i < len(list) {
			l = list[i]
		}
		if i < len(detail) {
			d = detail[i]
		}
		body = append(body, l+row(nil, 4)+d)
	}
	return m.renderFramed(w, h, []string{"PHP & Node"}, nil, strings.Join(body, "\n"))
}

func (m *Model) runtimeDetail(r runtimeRow, f runtimeFacts, running map[string]bool, w int) []string {
	if r.version == "" {
		return []string{row(nil, w, sp("Install a version with i", colDim))}
	}
	kv := func(k string, v ...seg) string {
		return row(nil, w, append([]seg{sp(padRight(k, 12), colDim)}, v...)...)
	}
	var out []string
	var users []string
	if r.kind == "php" {
		out = append(out, row(nil, w, bd("PHP "+r.version, nil)), row(nil, w))
		fpm := []seg{sp(glyphStopped+" ", colDim), sp("stopped", colDim)}
		if running[r.version] {
			fpm = []seg{sp(glyphRunning+" ", colRunning), sp("running", nil)}
		}
		out = append(out, kv("FPM", fpm...))
		out = append(out, kv("default", onOffSeg(r.version == f.defaultPHP)))
		xd := []seg{onOffSeg(f.xdebug[r.version])}
		if f.xdebug[r.version] && f.xdebugMode[r.version] != "" {
			xd = append(xd, sp("  "+f.xdebugMode[r.version], colDim))
		}
		out = append(out, kv("Xdebug", xd...))
		if e := f.extras[r.version]; e != "" {
			out = append(out, kv("extras", sp(e, nil)))
		}
		for _, s := range m.snap.Sites {
			if s.PHPVersion == r.version {
				users = append(users, siteDomain(&s))
			}
		}
	} else {
		out = append(out, row(nil, w, bd("Node "+r.version, nil)), row(nil, w))
		out = append(out, kv("default", onOffSeg(r.version == f.defaultNode)))
		for _, s := range m.snap.Sites {
			if s.NodeVersion == r.version {
				users = append(users, siteDomain(&s))
			}
		}
	}
	out = append(out, row(nil, w), row(nil, w, sideLabel(fmt.Sprintf("Sites using it   %d", len(users)))))
	if len(users) == 0 {
		out = append(out, row(nil, w, sp("none", colDim)))
	}
	for _, u := range users {
		out = append(out, row(nil, w, sp("· ", colDim), sp(u, nil)))
	}
	return out
}

func onOffSeg(on bool) seg {
	if on {
		return sp("yes", colRunning)
	}
	return sp("no", colDim)
}

// handleRuntimeKey owns the PHP & Node view's keys while it has focus.
func (m *Model) handleRuntimeKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.activeTab != tabRuntimes || m.sideFocus {
		return nil, false
	}
	r, ok := m.currentRuntime()
	switch msg.String() {
	case "up", "k":
		m.rtCursor = max(0, m.rtCursor-1)
		return nil, true
	case "down", "j":
		m.rtCursor = clamp(m.rtCursor+1, 0, max(0, len(m.runtimeRows())-1))
		return nil, true
	case "i":
		if r.kind == "node" {
			m.openPaletteIn("", "node:install ")
		} else {
			m.openPaletteIn("", "use ")
		}
		return nil, true
	}
	if !ok {
		return nil, false
	}
	switch msg.String() {
	case "d":
		if r.kind == "node" {
			m.setStatus("making Node "+r.version+" the default…", 10*time.Second)
			return tea.Sequence(runLerd("", "node:use", r.version), loadCmd()), true
		}
		m.setStatus("making PHP "+r.version+" the default…", 10*time.Second)
		return tea.Sequence(runLerd("", "use", r.version), loadCmd()), true
	case "x":
		if r.kind != "php" {
			return nil, true
		}
		verb := "on"
		if loadRuntimeFacts().xdebug[r.version] {
			verb = "off"
		}
		m.setStatus("xdebug "+verb+" PHP "+r.version+"…", 5*time.Second)
		return tea.Sequence(runLerd("", "xdebug", verb, r.version), loadCmd()), true
	case "R":
		if r.kind == "php" {
			m.openPaletteIn("", "php:rebuild "+r.version)
		}
		return nil, true
	}
	return nil, false
}
