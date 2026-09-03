package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dbview"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/stats"
	zone "github.com/lrstanley/bubblezone/v2"
)

// databasesMsg carries a finished engine listing back into the model.
type databasesMsg struct{ engines []dbview.Engine }

// databasesCmd introspects every installed engine off the main loop: listing
// databases execs a query inside each container, far too slow to run inline in
// a refresh tick.
func databasesCmd() tea.Cmd {
	return func() tea.Msg { return databasesMsg{engines: dbview.LoadAll()} }
}

// ensureDatabases loads the engine listing the first time the Databases tab is
// shown, and after a manual refresh has cleared it. Everywhere else the pane
// renders from the cached result, so moving the cursor costs nothing.
func (m *Model) ensureDatabases() tea.Cmd {
	if m.dbLoaded {
		return nil
	}
	return m.reloadDatabases()
}

// reloadDatabases re-lists the engines even when a listing is already held, for
// a manual refresh and after an action that changed what the pane shows. The
// held listing stays on screen until the new one lands, so the pane doesn't
// blank out for the second the queries take.
func (m *Model) reloadDatabases() tea.Cmd {
	if m.activeTab != tabDatabases || m.dbLoading {
		return nil
	}
	m.dbLoading = true
	return databasesCmd()
}

// dbRow is one line in the Databases list: an engine header (database < 0) or
// one of its databases. Only database rows are navigable, the same way the
// worktree headers in the site detail are captions rather than controls.
type dbRow struct {
	engine   int
	database int
}

// dbRows flattens the loaded engines into the row order the pane renders, so
// the cursor and the drawing walk exactly the same list.
func (m *Model) dbRows() []dbRow {
	var rows []dbRow
	for ei, eng := range m.dbEngines {
		rows = append(rows, dbRow{engine: ei, database: -1})
		for di := range eng.Databases {
			rows = append(rows, dbRow{engine: ei, database: di})
		}
	}
	return rows
}

// navigableDBRows returns the positions of the database rows, the ones the
// cursor may land on.
func navigableDBRows(rows []dbRow) []int {
	var idx []int
	for i, r := range rows {
		if r.database >= 0 {
			idx = append(idx, i)
		}
	}
	return idx
}

// currentDatabase resolves the selected engine and database, or nils when
// nothing is selected (no engines installed, or every engine is empty).
func (m *Model) currentDatabase() (*dbview.Engine, *dbview.Entry) {
	rows := m.dbRows()
	nav := navigableDBRows(rows)
	if len(nav) == 0 {
		return nil, nil
	}
	pos := clamp(m.dbCursor, 0, len(nav)-1)
	row := rows[nav[pos]]
	eng := &m.dbEngines[row.engine]
	return eng, &eng.Databases[row.database]
}

// renderDatabases draws the engines list: a header per engine, then its
// databases with size, owning site and snapshot count.
func (m *Model) renderDatabases(w, h int) string {
	style := paneStyle(m.focus == paneDatabases)
	innerW, innerH := innerSize(style, w, h)

	rows := m.dbRows()
	nav := navigableDBRows(rows)
	title := fmt.Sprintf("Databases (%d)", len(nav))
	lines := []string{padToWidth(clipLine(sectionStyle.Render(title), innerW), innerW)}

	availRows := innerH - len(lines)
	if availRows < 1 {
		availRows = 1
	}
	contentW := innerW - 1
	if contentW < 10 {
		contentW = innerW
	}

	var rowData []string
	cursorLine := 0
	switch {
	case m.dbLoading && !m.dbLoaded:
		rowData = []string{padToWidth(dimStyle.Render("listing engines…"), contentW)}
	case len(m.dbEngines) == 0:
		rowData = []string{
			padToWidth(dimStyle.Render("no database engine installed"), contentW),
			padToWidth("", contentW),
			padToWidth(dimStyle.Render("  install one with ")+accentStyle.Render("lerd preset install mysql"), contentW),
		}
	default:
		selected := -1
		if len(nav) > 0 {
			m.dbCursor = clamp(m.dbCursor, 0, len(nav)-1)
			selected = nav[m.dbCursor]
		}
		navPos := 0
		for i, r := range rows {
			eng := m.dbEngines[r.engine]
			if r.database < 0 {
				rowData = append(rowData, padToWidth(renderDBEngineRow(eng, contentW), contentW))
				continue
			}
			if i == selected {
				cursorLine = len(rowData)
			}
			row := padToWidth(renderDBRow(i == selected && m.focus == paneDatabases, eng.Databases[r.database], contentW), contentW)
			rowData = append(rowData, zone.Mark(fmt.Sprintf("db:%d", navPos), row))
			navPos++
		}
	}

	cur := -1
	if m.focus == paneDatabases && m.followCursor {
		cur = cursorLine
	}
	visible := viewport(rowData, cur, availRows, &m.dbScroll)
	bar := renderScrollbar(availRows, len(rowData), m.dbScroll, len(visible))
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

// renderDBEngineRow draws an engine header: its state dot, name, and the reason
// it lists nothing when it lists nothing.
func renderDBEngineRow(eng dbview.Engine, paneW int) string {
	glyph := stoppedStyle.Render(glyphStopped)
	note := strings.TrimSpace(dimStyle.Render("stopped"))
	if eng.Running {
		glyph = runningStyle.Render(glyphRunning)
		note = dimStyle.Render(fmt.Sprintf("%d", len(eng.Databases)))
	}
	if eng.Error != "" {
		glyph = failingStyle.Render(glyphFailing)
		note = failingStyle.Render("unreadable")
	}
	return clipLine(" "+glyph+" "+sectionStyle.Render(eng.Service)+"  "+note, paneW)
}

// dbNameColWidth aligns the size column across database rows. 22 cells fit a
// typical project database name without truncating.
const dbNameColWidth = 22

func renderDBRow(selected bool, db dbview.Entry, paneW int) string {
	prefix := "   "
	if selected {
		prefix = "  " + accentStyle.Render("▸")
	}
	name := padRight(truncatePlain(db.Name, dbNameColWidth), dbNameColWidth)
	if selected {
		name = selectedStyle.Render(name)
	}
	meta := dimStyle.Render(stats.FormatBytes(db.SizeBytes))
	if n := len(db.Snapshots); n > 0 {
		meta += dimStyle.Render(fmt.Sprintf("  %d snap", n))
	}
	return clipLine(prefix+" "+name+" "+meta, paneW)
}

// databaseDetailContentLines renders the right-hand pane on the Databases tab:
// the selected database, the site it belongs to, and every snapshot it holds.
// Only taking a snapshot is offered here; restore, drop, import and export
// overwrite or destroy data and stay in the CLI.
func databaseDetailContentLines(m *Model, innerW int) []string {
	out := make([]string, 0, 24)
	add := func(s string) { out = append(out, padToWidth(clipLine(s, innerW), innerW)) }

	eng, db := m.currentDatabase()
	if db == nil {
		add(sectionStyle.Render("Database detail"))
		if m.dbLoading && !m.dbLoaded {
			add(dimStyle.Render("  listing engines…"))
			return out
		}
		add(dimStyle.Render("  no database selected"))
		return out
	}

	add(sectionStyle.Render(db.Name))
	add(dimStyle.Render("  engine:  ") + eng.Service)
	add(dimStyle.Render("  size:    ") + stats.FormatBytes(db.SizeBytes))
	switch {
	case db.Owner.Branch != "":
		add(dimStyle.Render("  site:    ") + db.Owner.Domain + dimStyle.Render("  branch ") + db.Owner.Branch)
	case db.Owner.Domain != "":
		add(dimStyle.Render("  site:    ") + db.Owner.Domain)
	default:
		add(dimStyle.Render("  site:    ") + dimStyle.Render("no linked site uses it"))
	}
	add("")

	add(sectionStyle.Render("Snapshots"))
	switch {
	case !eng.SupportsSnapshot:
		add(dimStyle.Render("  " + eng.Service + " declares no snapshots"))
	case len(db.Snapshots) == 0:
		add(dimStyle.Render("  none yet, press ") + accentStyle.Render("n") + dimStyle.Render(" to take one"))
	default:
		cfg, _ := config.LoadGlobal()
		expiries := serviceops.SnapshotExpiries(serviceops.RetentionPolicy{
			Keep:    cfg.AutoSnapshotKeep(),
			KeepFor: cfg.AutoSnapshotKeepFor(),
			Every:   cfg.AutoSnapshotEvery(),
		}, db.Snapshots)
		now := time.Now()
		for i, s := range db.Snapshots {
			line := "  " + accentStyle.Render("·") + " " + padRight(truncatePlain(s.Name, 24), 24) + " "
			line += dimStyle.Render(s.Created.Local().Format("2006-01-02 15:04") + "  " + stats.FormatBytes(s.SizeBytes))
			if s.GitBranch != "" {
				line += dimStyle.Render("  " + s.GitBranch)
			}
			if s.Auto {
				line += dimStyle.Render("  auto")
				if label := expiries[i].Label(now); label != "" {
					line += dimStyle.Render(" " + label)
				}
			}
			add(line)
		}
	}
	add("")

	add(sectionStyle.Render("Actions"))
	add(dimStyle.Render("  n snapshot   K keep an automatic snapshot"))
	add(dimStyle.Render("  restore, drop, import and export overwrite data and live in the CLI:"))
	add(dimStyle.Render("  ") + accentStyle.Render("lerd db:restore") + dimStyle.Render(" · ") + accentStyle.Render("lerd db:import") + dimStyle.Render(" · ") + accentStyle.Render("lerd db:export"))
	return out
}

// actionDatabaseSnapshot takes a snapshot of the selected database through the
// same CLI verb a user would type. Adding a snapshot takes nothing away, which
// is why it is the one database action the TUI runs.
func (m *Model) actionDatabaseSnapshot() tea.Cmd {
	eng, db := m.currentDatabase()
	if db == nil {
		return nil
	}
	if !eng.SupportsSnapshot {
		m.setStatus(eng.Service+" declares no snapshots", 3*time.Second)
		return nil
	}
	m.setStatus("snapshotting "+db.Name+"…", 10*time.Second)
	return runLerd(databaseSnapshotDir(db.Owner), "db:snapshot", "--service", eng.Service, "--database", db.Name)
}

// databaseSnapshotDir is the directory db:snapshot runs in, which is what the
// snapshot records as its site and git branch. Running it from the owning
// project keeps that context true; with no known owner it runs from the home
// dir, so whatever repo the TUI happens to be launched in cannot label a
// snapshot with an unrelated branch.
func databaseSnapshotDir(owner dbview.Owner) string {
	if owner.Domain != "" {
		if site, err := config.FindSiteByDomain(owner.Domain); err == nil && site.Path != "" {
			return site.Path
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
