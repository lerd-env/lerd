package tui

import (
	"charm.land/lipgloss/v2"
	"fmt"
	"github.com/geodro/lerd/internal/siteinfo"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	return func() tea.Msg {
		wakeSleepingEngines()
		return databasesMsg{engines: withoutStreamingHidden(dbview.LoadAll())}
	}
}

// wakeSleepingEngines brings back the database engines idle-suspend put to
// sleep, together, before the tab lists them: looking at databases is using
// the engines, and a sleeping one would otherwise read as stopped.
func wakeSleepingEngines() {
	var wg sync.WaitGroup
	for _, name := range config.IdleSuspendedServices() {
		if !serviceops.DeclaresDatabases(name) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tuiWakeService(name)
		}()
	}
	wg.Wait()
}

// withoutStreamingHidden drops the databases a site streaming mode hides owns.
func withoutStreamingHidden(engines []dbview.Engine) []dbview.Engine {
	cfg, _ := config.LoadGlobal()
	reg, err := config.LoadSites()
	if err != nil {
		return engines
	}
	hidden := cfg.StreamingHidden(reg)
	domains := config.HiddenDomains(reg, hidden)
	if len(hidden) == 0 {
		return engines
	}
	for i := range engines {
		kept := []dbview.Entry{}
		for _, db := range engines[i].Databases {
			if !config.EntityHidden(db.Name, db.Owner.Domain, hidden, domains) {
				kept = append(kept, db)
			}
		}
		engines[i].Databases = kept
	}
	return engines
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
	testing  int // index of the folded "<name>_testing" sibling, -1 when none
}

// dbRows flattens the loaded engines into the row order the pane renders, so
// the cursor and the drawing walk exactly the same list. A "<name>_testing"
// database folds into the row of "<name>", as the web UI folds it into that
// card; one whose app database is missing keeps a row of its own.
func (m *Model) dbRows() []dbRow {
	var rows []dbRow
	for ei, eng := range m.dbEngines {
		rows = append(rows, dbRow{engine: ei, database: -1, testing: -1})
		index := make(map[string]int, len(eng.Databases))
		for di, db := range eng.Databases {
			index[db.Name] = di
		}
		for di, db := range eng.Databases {
			if base, ok := strings.CutSuffix(db.Name, dbview.TestingSuffix); ok {
				if _, paired := index[base]; paired {
					continue
				}
			}
			testing := -1
			if ti, ok := index[db.Name+dbview.TestingSuffix]; ok {
				testing = ti
			}
			rows = append(rows, dbRow{engine: ei, database: di, testing: testing})
		}
	}
	return rows
}

// currentTestingDatabase is the testing database folded into the selection.
func (m *Model) currentTestingDatabase() *dbview.Entry {
	rows := m.dbRows()
	nav := navigableDBRows(rows)
	if len(nav) == 0 {
		return nil
	}
	row := rows[nav[clamp(m.dbCursor, 0, len(nav)-1)]]
	if row.testing < 0 {
		return nil
	}
	return &m.dbEngines[row.engine].Databases[row.testing]
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

func (m *Model) renderDatabasesIn(style lipgloss.Style, w, h int) string {
	innerW, innerH := innerSize(style, w, h)

	rows := m.dbRows()
	nav := navigableDBRows(rows)
	// The bordered pane titles itself; inside the view the breadcrumb does.
	var lines []string
	if style.GetHorizontalFrameSize() > 0 {
		title := fmt.Sprintf("Databases (%d)", len(nav))
		lines = []string{padToWidth(clipLine(sectionStyle.Render(title), innerW), innerW)}
	}

	availRows := innerH - len(lines)
	if availRows < 1 {
		availRows = 1
	}
	contentW := innerW - 2 // a gap and the scrollbar
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
			padToWidth(dimStyle.Render("  add one with A in Services, or ")+accentStyle.Render("lerd service preset mysql"), contentW),
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
				shown := 0
				for _, other := range rows {
					if other.engine == r.engine && other.database >= 0 {
						shown++
					}
				}
				rowData = append(rowData, padToWidth(renderDBEngineRow(eng, shown, contentW), contentW))
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
// shown is the number of rows listed under it, after testing databases fold in.
func renderDBEngineRow(eng dbview.Engine, shown, paneW int) string {
	glyph := stoppedStyle.Render(glyphStopped)
	note := strings.TrimSpace(dimStyle.Render("stopped"))
	if eng.Running {
		glyph = runningStyle.Render(glyphRunning)
		note = dimStyle.Render(fmt.Sprintf("%d", shown))
	}
	if eng.Error != "" {
		glyph = failingStyle.Render(glyphFailing)
		note = failingStyle.Render("unreadable")
	}
	return clipLine(glyph+" "+sectionStyle.Render(eng.Service)+"  "+note, paneW)
}

// dbNameColWidth aligns the size column across database rows. 22 cells fit a
// typical project database name without truncating.
const dbNameColWidth = 22

func renderDBRow(selected bool, db dbview.Entry, paneW int) string {
	prefix := "  "
	if selected {
		prefix = accentStyle.Render("▸") + " "
	}
	name := padRight(truncatePlain(db.Name, dbNameColWidth), dbNameColWidth)
	if selected {
		name = selectedStyle.Render(name)
	}
	// A row carries the name and size only; snapshots and the folded testing
	// database belong to the detail pane, where there is room to read them.
	return clipLine(prefix+name+" "+dimStyle.Render(fmt.Sprintf("%7s", stats.FormatBytes(db.SizeBytes))), paneW)
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
	if site := m.dbOwnerSite(db); site != nil {
		state := dimStyle.Render("off for " + site.Name)
		if autoSnapshotCovered(site.AutoSnapshot) {
			state = runningStyle.Render("on") + dimStyle.Render(" for "+site.Name)
		}
		add(dimStyle.Render("  auto:    ") + state)
	}
	add("")

	if t := m.currentTestingDatabase(); t != nil {
		add(sectionStyle.Render("Testing database"))
		add(dimStyle.Render("  name:    ") + t.Name)
		add(dimStyle.Render("  size:    ") + stats.FormatBytes(t.SizeBytes))
		add(dimStyle.Render("  snaps:   ") + fmt.Sprintf("%d", len(t.Snapshots)))
		add("")
	}

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

	add(dimStyle.Render("  restore, drop and import overwrite data, so they stay in the CLI:"))
	add("  " + accentStyle.Render("lerd db:restore") + dimStyle.Render(" · ") + accentStyle.Render("lerd db:import"))
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

// dbOwnerSite is the linked site the selected database belongs to, if any.
func (m *Model) dbOwnerSite(db *dbview.Entry) *siteinfo.EnrichedSite {
	if db == nil || db.Owner.Domain == "" {
		return nil
	}
	for i := range m.snap.Sites {
		for _, d := range m.snap.Sites[i].Domains {
			if d == db.Owner.Domain {
				return &m.snap.Sites[i]
			}
		}
	}
	return nil
}

// handleDatabaseKey owns the Databases view's three actions that add or change
// nothing destructive: create a database, export one to a file, and put its
// site on or off the automatic snapshot schedule.
func (m *Model) handleDatabaseKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.activeTab != tabDatabases {
		return nil, false
	}
	eng, db := m.currentDatabase()
	switch msg.String() {
	case "c":
		service := ""
		if eng != nil {
			service = "--service " + eng.Service + " "
		} else if len(m.dbEngines) > 0 {
			service = "--service " + m.dbEngines[0].Service + " "
		}
		m.openPaletteIn("", "db:create "+service)
		return nil, true
	case "e":
		if db == nil {
			return nil, true
		}
		dir := databaseSnapshotDir(db.Owner)
		if site := m.dbOwnerSite(db); site != nil && site.Path != "" {
			dir = site.Path
		}
		out := filepath.Join(dir, db.Name+".sql")
		m.setStatus("exporting "+db.Name+" to "+out+"…", 30*time.Second)
		return runLerd(dir, "db:export", "--service", eng.Service, "--database", db.Name, "--output", out), true
	case "a":
		if db == nil {
			return nil, true
		}
		site := m.dbOwnerSite(db)
		if site == nil {
			m.setStatus("no site owns "+db.Name+", so it has no snapshot schedule to join", 4*time.Second)
			return nil, true
		}
		if autoSnapshotCovered(site.AutoSnapshot) {
			m.setStatus("excluding "+site.Name+" from automatic snapshots…", 5*time.Second)
			return tea.Sequence(runLerd(site.Path, "db:snapshot:auto", "site", site.Name, "off"), loadCmd(), m.reloadDatabases()), true
		}
		m.setStatus("including "+site.Name+" in automatic snapshots…", 5*time.Second)
		return tea.Sequence(runLerd(site.Path, "db:snapshot:auto", "site", site.Name, "on"), loadCmd(), m.reloadDatabases()), true
	}
	return nil, false
}
