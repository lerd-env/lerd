package tui

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/shims"
	"github.com/geodro/lerd/internal/stats"
)

// presetSuggestions mirrors internal/ui/web/src/stores/presetSuggestions.ts:
// a service of the key gets an "install <value> for an admin dashboard"
// hint. The TUI repeats the same map so users see the same nudge on both
// surfaces; eventually this should move into Go config land but keeping a
// local copy avoids a Phase-4-only refactor of an unrelated module.
var presetSuggestions = map[string]string{
	"mysql":         "phpmyadmin",
	"postgres":      "pgadmin",
	"mongo":         "mongo-express",
	"elasticsearch": "elasticvue",
	"typesense":     "typesense-dashboard",
}

// serviceDetailContentLines renders the right-hand pane when focus is on
// the services list. Mirrors the web UI's ServiceDetail layout: a header,
// the running state, dependencies, a per-service env block, the list of
// sites referencing it, and a preset-suggestion banner where one applies.
// Worker rows (queue-X, schedule-X, …) get their own variant since they
// have no own container or env.
func serviceDetailContentLines(m *Model, svc *ServiceRow, innerW int) []string {
	lines, _ := serviceDetailContentLinesWithCursor(m, svc, innerW)
	return lines
}

// serviceDetailContentLinesWithCursor is serviceDetailContentLines plus the
// line index of the selected client-tool row, so the pane can scroll the
// selection into view the way the site detail does.
func serviceDetailContentLinesWithCursor(m *Model, svc *ServiceRow, innerW int) ([]string, int) {
	out := make([]string, 0, 32)
	cursorLine := -1
	add := func(s string) { out = append(out, padToWidth(clipLine(s, innerW), innerW)) }

	if svc == nil {
		add(sectionStyle.Render("Service detail"))
		add(dimStyle.Render("  no service selected"))
		return out, cursorLine
	}

	if svc.WorkerKind != "" {
		return workerDetailContentLines(svc, innerW), cursorLine
	}

	// Header: name, version, state.
	add(sectionStyle.Render(svc.Name))
	stateText := serviceStateText(svc.State)
	if svc.Version != "" {
		add(dimStyle.Render("  version: ") + svc.Version)
	}
	add(dimStyle.Render("  state:   ") + stateText)
	add(dimStyle.Render("  unit:    ") + "lerd-" + svc.Name)
	// Published host port and any extra mappings, read-only. Editing lives in
	// the CLI/UI/MCP (a multi-field edit is out of the TUI's quick-action scope).
	if host, def, extras := servicePortsInfo(svc.Name); host > 0 {
		portLine := strconv.Itoa(host)
		if def > 0 && host != def {
			portLine += dimStyle.Render(" (default " + strconv.Itoa(def) + ")")
		}
		add(dimStyle.Render("  ports:   ") + portLine)
		for _, e := range extras {
			add(dimStyle.Render("  +extra:  ") + e)
		}
	}
	if svc.Pinned {
		add(dimStyle.Render("  pinned:  ") + accentStyle.Render("yes (preset will not auto-update)"))
	}
	if svc.Dashboard != "" {
		add(dimStyle.Render("  dashbd:  ") + svc.Dashboard + dimStyle.Render("  (") + accentStyle.Render("O") + dimStyle.Render(" to open)"))
	}
	add("")

	// Dependencies.
	if len(svc.DependsOn) > 0 {
		add(sectionStyle.Render("Depends on"))
		states := m.serviceStatesByName()
		for _, dep := range svc.DependsOn {
			label := serviceops.DependencyDisplayName(dep)
			key := serviceops.ResolveDependency(dep)
			if key == "" {
				key = dep
			}
			add(renderSiteServiceRow(label, states[key]))
		}
		add("")
	}

	// Sites using this service.
	add(sectionStyle.Render("Sites using"))
	sites := config.SitesUsingService(svc.Name)
	if len(sites) == 0 {
		add(dimStyle.Render("  no sites currently reference " + svc.Name))
	} else {
		for _, s := range sites {
			add("  " + accentStyle.Render("·") + " " + s.Name)
		}
	}
	add("")

	// Env vars (templates from the preset or env from a custom service).
	envLines := serviceEnvLines(svc.Name, svc.Custom)
	if len(envLines) > 0 {
		add(sectionStyle.Render("Env vars"))
		for _, ln := range envLines {
			add("  " + dimStyle.Render(ln))
		}
		add("")
	}

	// Client tools: the host shims this service exposes, each a reversible
	// toggle (writing or removing a file in the bin dir), so they sit with
	// start / stop rather than behind the CLI. A tool another installed
	// service owns is listed but managed from that service.
	tools := shims.ServiceShims(svc.Name)
	if len(tools) > 0 {
		add(sectionStyle.Render("Client tools"))
		nav := navigableShimRows(tools, svc.Name)
		m.clampServiceDetailCursor(len(nav))
		selected := -1
		if len(nav) > 0 && m.serviceDetailFocused() {
			selected = nav[m.svcDetailCursor]
		}
		for i, info := range tools {
			if i == selected {
				cursorLine = len(out)
			}
			add(renderShimRow(info, svc.Name, i == selected))
		}
		add("")
	}

	// Tuning: the values actually in effect, read off the override file the
	// Config surface writes. Editing is a whole-file edit, so it stays in
	// `lerd service config`; this pane only reports what is set.
	if target, values, ok := serviceTuningInfo(svc.Name); ok {
		add(sectionStyle.Render("Tuning"))
		add(dimStyle.Render("  file:    ") + target)
		if len(values) == 0 {
			add(dimStyle.Render("  no overrides set, the image defaults are in effect"))
		} else {
			for _, v := range values {
				add("  " + v)
			}
		}
		add(dimStyle.Render("  edit with ") + accentStyle.Render("lerd service config "+svc.Name))
		add("")
	}

	// Entities: whatever the preset declares it holds (buckets, indexes,
	// collections). Listing execs in the container, so it arrives
	// asynchronously and is cached per service; creating or dropping one
	// stays with the CLI.
	out = append(out, serviceEntityLines(m, svc, innerW)...)

	// Preset suggestion banner: if the focused service has an associated
	// admin dashboard preset that isn't installed yet, hint at it. We don't
	// install from the TUI (Preset install is destructive-ish per the TUI
	// scope rule); the banner just points the user at the CLI verb.
	if hint := presetSuggestionFor(svc); hint != "" {
		add(accentStyle.Render("  💡 ") + hint)
		add("")
	}

	// Quick-action hint so the user discovers what's reversible from the
	// services pane: matches what the help reference says.
	add(sectionStyle.Render("Actions"))
	actions := "  s start · x stop · r restart · t shell · u update · b rollback · l logs"
	if svc.Dashboard != "" {
		actions += " · O dashboard"
	}
	if len(tools) > 0 {
		actions += " · space toggle client tool"
	}
	add(dimStyle.Render(actions))
	return out, cursorLine
}

// navigableShimRows returns the indexes of the client-tool rows this service
// may toggle: a tool a different installed service owns is shown for context
// but is managed from its owner, mirroring the web UI's disabled toggle.
func navigableShimRows(tools []shims.Info, service string) []int {
	var idx []int
	for i, info := range tools {
		if info.Owner != "" && info.Owner != service {
			continue
		}
		idx = append(idx, i)
	}
	return idx
}

// renderShimRow draws one client-tool row: state glyph, tool name, on/off, and
// the note that explains why a row cannot be toggled or what turning it on
// would shadow.
func renderShimRow(info shims.Info, service string, selected bool) string {
	prefix := "  "
	if selected {
		prefix = " " + accentStyle.Render("▸")
	}
	glyph := stoppedStyle.Render(glyphStopped)
	state := strings.TrimSpace(dimStyle.Render("off"))
	if info.Enabled {
		glyph = runningStyle.Render(glyphRunning)
		state = runningStyle.Render("on")
	}
	name := padRight(truncatePlain(info.Tool, 16), 16)
	if selected {
		name = selectedStyle.Render(name)
	}
	note := ""
	switch {
	case info.Owner != "" && info.Owner != service:
		note = dimStyle.Render("  provided by " + info.Owner)
	case info.HostHas:
		note = suspendedStyle.Render("  shadows your own " + info.Tool)
	}
	return prefix + " " + glyph + " " + name + " " + state + note
}

// serviceTuningInfo returns the in-container path a service's tuning override
// is mounted at and the settings currently set in it, comments and blank lines
// dropped so the pane shows what is actually in effect. ok is false for a
// service that declares no tuning at all.
func serviceTuningInfo(name string) (target string, values []string, ok bool) {
	svc, err := config.ResolveServiceForTuning(name)
	if err != nil || svc == nil {
		return "", nil, false
	}
	target, ok = config.ServiceTuningMount(svc)
	if !ok {
		return "", nil, false
	}
	data, err := os.ReadFile(config.ServiceTuningFile(name))
	if err != nil {
		return target, nil, true
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		values = append(values, line)
	}
	return target, values, true
}

// serviceEntityLines renders the cached entity listing for a service: one block
// per declared kind with its rows, the declared columns beside each name. The
// listing itself is fetched by ensureServiceEntities.
func serviceEntityLines(m *Model, svc *ServiceRow, innerW int) []string {
	kinds, cached := m.svcEntities[svc.Name]
	if !cached && m.svcEntitiesLoading != svc.Name {
		return nil
	}
	out := make([]string, 0, 8)
	add := func(s string) { out = append(out, padToWidth(clipLine(s, innerW), innerW)) }
	if !cached {
		add(sectionStyle.Render("Entities"))
		add(dimStyle.Render("  listing…"))
		add("")
		return out
	}
	if len(kinds) == 0 {
		return nil
	}
	add(sectionStyle.Render("Entities"))
	for _, k := range kinds {
		label := k.label
		if label == "" {
			label = k.kind
		}
		add("  " + label + dimStyle.Render(" ("+strconv.Itoa(len(k.rows))+")"))
		if k.err != "" {
			add(failingStyle.Render("    " + k.err))
			continue
		}
		if len(k.rows) == 0 {
			add(dimStyle.Render("    none"))
			continue
		}
		for _, row := range k.rows {
			line := "    " + accentStyle.Render("·") + " " + row.Name
			if cols := entityRowMeta(k, row); cols != "" {
				line += dimStyle.Render("  " + cols)
			}
			add(line)
		}
	}
	add(dimStyle.Render("  create and drop live in the CLI"))
	add("")
	return out
}

// entityRowMeta joins a row's declared column values into one dim trailer,
// formatting byte counts the way the dashboard does so a size reads as a size.
func entityRowMeta(k serviceEntityKind, row serviceops.EntityRow) string {
	parts := make([]string, 0, len(row.Values))
	for i, v := range row.Values {
		if v == "" || i >= len(k.columns) {
			continue
		}
		col := k.columns[i]
		if col.format == "bytes" {
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				v = stats.FormatBytes(n)
			}
		}
		label := col.label
		if label == "" {
			label = col.key
		}
		parts = append(parts, label+" "+v)
	}
	return strings.Join(parts, " · ")
}

// serviceEntityKind is one declared entity kind with the rows listed inside the
// service. Cached on the model per service so the container exec runs on
// selection rather than on every frame.
type serviceEntityKind struct {
	kind    string
	label   string
	columns []serviceEntityColumn
	rows    []serviceops.EntityRow
	err     string
}

type serviceEntityColumn struct {
	key    string
	label  string
	format string
}

// serviceEntitiesMsg carries a finished entity listing back into the model,
// keyed by the service it was run for so a late result cannot land against
// another service.
type serviceEntitiesMsg struct {
	service string
	kinds   []serviceEntityKind
}

// serviceEntitiesCmd lists every declared entity kind of a service off the main
// loop: each kind runs its declared list command inside the container, which is
// far too slow to do inline in a render.
func serviceEntitiesCmd(service string) tea.Cmd {
	return func() tea.Msg {
		specs := serviceops.ServiceEntities(service)
		kinds := make([]serviceEntityKind, 0, len(specs))
		for i := range specs {
			spec := &specs[i]
			k := serviceEntityKind{kind: spec.Kind, label: spec.Label}
			for _, c := range spec.Columns {
				k.columns = append(k.columns, serviceEntityColumn{key: c.Key, label: c.Label, format: c.Format})
			}
			rows, err := serviceops.ListEntities(service, spec)
			if err != nil {
				k.err = err.Error()
			} else {
				k.rows = rows
			}
			kinds = append(kinds, k)
		}
		return serviceEntitiesMsg{service: service, kinds: kinds}
	}
}

// ensureServiceEntities kicks off the entity listing for the selected service
// when it has none cached yet. Only a running service is asked: the list
// commands exec in its container.
func (m *Model) ensureServiceEntities() tea.Cmd {
	if m.activeTab != tabServices || m.svcEntitiesLoading != "" {
		return nil
	}
	svc := m.currentService()
	// A sleeping service is asked too: listing what it holds wakes it.
	if svc == nil || svc.WorkerKind != "" || (svc.State != stateRunning && svc.State != stateSuspended) {
		return nil
	}
	if _, ok := m.svcEntities[svc.Name]; ok {
		return nil
	}
	m.svcEntitiesLoading = svc.Name
	return serviceEntitiesCmd(svc.Name)
}

// serviceDetailFocused reports whether the client-tool cursor is live: the
// service detail pane owns focus on the Services tab.
func (m *Model) serviceDetailFocused() bool {
	return m.activeTab == tabServices && m.focus == paneDetail
}

// clampServiceDetailCursor keeps the client-tool cursor inside the rows the
// current service actually has.
func (m *Model) clampServiceDetailCursor(n int) {
	m.svcDetailCursor = clamp(m.svcDetailCursor, 0, max(0, n-1))
}

// serviceShimNavCount is how many client-tool rows the selected service can
// toggle, the bound cursor movement clamps against.
func (m *Model) serviceShimNavCount() int {
	svc := m.currentService()
	if svc == nil || svc.WorkerKind != "" {
		return 0
	}
	return len(navigableShimRows(shims.ServiceShims(svc.Name), svc.Name))
}

// toggleServiceShim installs or removes the selected client tool's host shim,
// through the same CLI verb a user would type.
func (m *Model) toggleServiceShim() tea.Cmd {
	svc := m.currentService()
	if svc == nil || svc.WorkerKind != "" {
		return nil
	}
	tools := shims.ServiceShims(svc.Name)
	nav := navigableShimRows(tools, svc.Name)
	if len(nav) == 0 {
		return nil
	}
	m.clampServiceDetailCursor(len(nav))
	info := tools[nav[m.svcDetailCursor]]
	if info.Enabled {
		m.setStatus("removing the "+info.Tool+" shim…", 5*time.Second)
		return runLerd("", "shims", "remove", info.Tool)
	}
	m.setStatus("installing the "+info.Tool+" shim…", 5*time.Second)
	return runLerd("", "shims", "add", info.Tool)
}

// workerDetailContentLines renders the service-detail pane variant for
// worker rows (queue-X, schedule-X, custom-X). Workers run as systemd user
// units inside the owning site's FPM container, so their detail differs
// from regular services: there's no image, no DependsOn, no preset
// suggestion — just the parent site, kind, and unit name.
func workerDetailContentLines(svc *ServiceRow, innerW int) []string {
	out := make([]string, 0, 16)
	add := func(s string) { out = append(out, padToWidth(clipLine(s, innerW), innerW)) }

	add(sectionStyle.Render(svc.Name))
	add(dimStyle.Render("  kind:    ") + svc.WorkerKind)
	add(dimStyle.Render("  site:    ") + svc.WorkerSite)
	add(dimStyle.Render("  state:   ") + serviceStateText(svc.State))
	add(dimStyle.Render("  unit:    ") + "lerd-" + svc.WorkerKind + "-" + svc.WorkerSite)
	if svc.WorkerPath != "" {
		add(dimStyle.Render("  path:    ") + svc.WorkerPath)
	}
	add("")
	add(sectionStyle.Render("Actions"))
	add(dimStyle.Render("  s start · x stop · r restart · t shell (parent site container) · l logs"))
	return out
}

// serviceEnvLines returns the env-var entries declared by a service's
// preset or its custom-service YAML. Mirrors the union of what the web
// UI's ServiceEnvTab and PHP-side env writer see. Lines come back already
// trimmed to "KEY=value" form, with Environment map keys sorted so the
// render is stable across redraws (Go map iteration is randomised, and
// the service-detail pane re-renders every spinner tick).
func serviceEnvLines(name string, custom bool) []string {
	if custom {
		svc, err := config.LoadCustomService(name)
		if err != nil || svc == nil {
			return nil
		}
		out := make([]string, 0, len(svc.EnvVars)+len(svc.Environment))
		out = append(out, svc.EnvVars...)
		keys := make([]string, 0, len(svc.Environment))
		for k := range svc.Environment {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, k+"="+svc.Environment[k])
		}
		return out
	}
	if config.IsDefaultPreset(name) {
		return config.DefaultPresetEnvVars(name)
	}
	return nil
}

// presetSuggestionFor returns a one-line nudge string when the focused
// service has an associated admin-dashboard preset the user hasn't
// installed yet. Returns "" when there's no suggestion or the admin
// service is already installed (detected via serviceops.ServiceInstalled).
func presetSuggestionFor(svc *ServiceRow) string {
	if svc == nil {
		return ""
	}
	target, ok := presetSuggestions[svc.Name]
	if !ok {
		return ""
	}
	if serviceops.ServiceInstalled(target) {
		return ""
	}
	return "install " + target + " for a browser dashboard (run `lerd preset install " + target + "`)"
}

// servicePortsInfo returns the host (published) port a built-in service is
// exposed on, its preset-default host port, and any extra published mappings,
// read straight from global config for the read-only TUI ports line. host is 0
// when the service publishes no host port (e.g. a custom service whose ports
// live in its own YAML), in which case the caller omits the line.
func servicePortsInfo(name string) (host, def int, extras []string) {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return 0, 0, nil
	}
	sc := cfg.Services[name]
	def = sc.Port
	host = def
	if sc.PublishedPort > 0 {
		host = sc.PublishedPort
	}
	return host, def, sc.ExtraPorts
}

// serviceStateText renders a one-word state with the matching colour. Used
// in the service header and the dependency rows; centralised so a future
// state rename only changes here.
func serviceStateText(state ServiceState) string {
	switch state {
	case stateRunning:
		return runningStyle.Render("running")
	case statePaused:
		return pausedStyle.Render("paused")
	case stateSuspended:
		return suspendedStyle.Render("suspended")
	default:
		return strings.TrimSpace(dimStyle.Render("stopped"))
	}
}
