package tui

// helpSection groups related keybindings under a heading.
type helpSection struct {
	title string
	rows  [][2]string // {key, description}
}

// helpReference is the canonical list of every keybinding in the TUI. It
// lives here rather than inline in panes.go so a future keymap overhaul
// (remappable keys, user-defined shortcuts) has one place to edit.
var helpReference = []helpSection{
	{
		title: "Navigation",
		rows: [][2]string{
			{"ctrl+← / ctrl+→", "switch the top tab (Dashboard · Sites · Services); tabs are also clickable"},
			{"tab / shift+tab", "cycle focus between the list and the detail pane on the current tab"},
			{"click", "click a tab to switch screens, or a site / service / worker row to open it"},
			{"↑ ↓  j k", "move the selection in the focused pane or Dashboard card (info cards scroll)"},
			{"enter", "on the Dashboard, open the selected row (same as clicking it)"},
			{"pgup / pgdn", "jump by 10 rows"},
			{"home / end · g G", "jump to first / last row"},
		},
	},
	{
		title: "Filter & sort",
		rows: [][2]string{
			{"/", "type to filter the focused list by name"},
			{"  enter", "apply filter and leave input mode"},
			{"  esc", "clear filter and leave input mode"},
			{"o", "cycle the focused list's sort order"},
			{"  sites", "name · status · framework · workspace"},
			{"  services", "name · status · usage"},
		},
	},
	{
		title: "Actions",
		rows: [][2]string{
			{"space / enter", "toggle the focused detail row (worker, HTTPS, LAN share, PHP, Node)"},
			{"s", "start / unpause the focused site or start the focused service"},
			{"x", "stop / pause the focused site or stop / remove the focused domain"},
			{"r", "restart the focused site or service"},
			{"p", "pause / unpause toggle for a site"},
			{"t", "open an interactive shell inside the focused container"},
			{"O", "open in the browser: the focused site's primary domain, or the focused service's dashboard URL"},
			{"u", "service update — pull a newer image and restart (services pane)"},
			{"b", "service rollback — revert to the previously-running image (services pane)"},
		},
	},
	{
		title: "Site detail tabs",
		rows: [][2]string{
			{"1", "Overview tab (default — workers, toggles, worktrees, app-logs pane beneath)"},
			{"2", "Env tab (read-only .env display)"},
			{"3", "Debug tab (this site's lenses: dumps, queries, jobs, mail, …)"},
			{"4", "Doctor tab (Laravel only — APP_KEY, env drift, migrations, storage link; press again to re-run)"},
			{"{ / }", "scroll the Overview app-logs pane (or the logs pane when open)"},
		},
	},
	{
		title: "Debug view",
		rows: [][2]string{
			{"[ / ]", "switch lens (Dumps · Queries · Jobs · Views · Mail · Cache · Events · HTTP)"},
			{"/", "search the active lens (site, request, worker, file, text, payload)"},
			{"1 / 2", "toggle the `fpm` / `cli` context-filter chips"},
			{"enter / space", "expand the selected row (bindings, caller, exception, …)"},
			{"w", "toggle worker capture (queue / scheduler events)"},
			{"c", "clear the in-memory buffer and run `lerd dump clear`"},
			{"T", "toggle the debug bridge globally (lerd dump on/off)"},
		},
	},
	{
		title: "Toasts",
		rows: [][2]string{
			{"d", "dismiss the newest toast in the bottom-right corner"},
		},
	},
	{
		title: "Domains",
		rows: [][2]string{
			{"a", "add a new domain to the focused site (inline input)"},
			{"e", "edit / rename the focused domain row (add new + remove old)"},
			{"x", "remove the focused domain"},
		},
	},
	{
		title: "Worktrees",
		rows: [][2]string{
			{"space / enter", "toggle the focused per-worktree row (worker, isolated DB)"},
		},
	},
	{
		title: "Logs",
		rows: [][2]string{
			{"l", "toggle the logs pane for the focused item"},
			{"[ / ]", "cycle through the site's log sources (FPM, workers, app logs)"},
			{"{ / }", "scroll back through buffered output / return to tail"},
			{"f", "find within the tailed buffer — matches highlighted, non-matches dimmed"},
		},
	},
	{
		title: "Panes & overlays",
		rows: [][2]string{
			{"Dashboard tab", "six-card overview (Sites · Services · Workers · System Health · Resources · Lerd)"},
			{"S", "swap the detail pane for global Settings (LAN expose, autostart, Xdebug) — Sites tab"},
			{"Y", "swap the detail pane for the System overview (DNS, Nginx, Watcher, PHP, Node, Lerd) — Sites tab"},
			{"D", "open the Debug window (dumps, queries with N+1, jobs, mail, …) — Sites tab"},
			{"?", "swap the detail pane for this help reference"},
			{"esc", "close picker or return to site detail"},
		},
	},
	{
		title: "General",
		rows: [][2]string{
			{":", "open the command palette — type any `lerd <args>` and press enter"},
			{"R", "force a manual refresh"},
			{"H", "heal every failed worker (reset-failed + start)"},
			{"q / ctrl+c", "quit"},
		},
	},
}

// helpContentLines builds the lines drawn inside the detail pane when
// detailMode == detailHelp. Same padding / clipping convention as the other
// detail renderers so the border wraps cleanly.
func helpContentLines(m *Model, innerW int) []string {
	out := make([]string, 0, 64)
	add := func(s string) { out = append(out, padToWidth(clipLine(s, innerW), innerW)) }

	add(sectionStyle.Render("Keybindings"))
	add(dimStyle.Render("  press ? or esc to return to site detail"))
	add("")

	for i, sec := range helpReference {
		if i > 0 {
			add("")
		}
		add(sectionStyle.Render(sec.title))
		for _, row := range sec.rows {
			key := padRight(truncatePlain(row[0], 18), 18)
			add("  " + accentStyle.Render(key) + "  " + row[1])
		}
	}
	return out
}
