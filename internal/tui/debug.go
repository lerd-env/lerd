package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/devtoolsops"
	lerddumps "github.com/geodro/lerd/internal/dumps"
)

// debugLenses are the Debug view's switchable lenses, in tab order, mirroring
// the web Debug window. Each maps a wire kind to a tab label; `[` / `]` cycle
// between them. Dumps render as a flat list (dump-bridge events carry no
// request id); every other kind groups by request like the dashboard does.
var debugLenses = []struct {
	kind, label string
}{
	{lerddumps.KindDump, "Dumps"},
	{lerddumps.KindQuery, "Queries"},
	{lerddumps.KindJob, "Jobs"},
	{lerddumps.KindView, "Views"},
	{lerddumps.KindMail, "Mail"},
	{lerddumps.KindCache, "Cache"},
	{lerddumps.KindEvent, "Events"},
	{lerddumps.KindHTTP, "HTTP"},
}

// inDebugView reports whether the Debug lenses are on screen: the global D
// view, or the per-site Debug tab. Lens-switch (`[`/`]`) and worker-toggle
// (`w`) keys route to the debug handlers in both places.
func (m *Model) inDebugView() bool {
	return m.detailMode == detailDumps ||
		(m.detailMode == detailSite && m.siteTab == tabSiteDebug)
}

func (m *Model) activeLensKind() string {
	if m.debugLens < 0 || m.debugLens >= len(debugLenses) {
		return lerddumps.KindDump
	}
	return debugLenses[m.debugLens].kind
}

// cycleDebugLens moves the active lens by delta (wrapping) and resets the
// cursor so the selection starts at the newest row of the new lens.
func (m *Model) cycleDebugLens(delta int) {
	n := len(debugLenses)
	m.debugLens = ((m.debugLens+delta)%n + n) % n
	m.dumpsCursor = 0
	m.dumpsScroll = 0
}

// toggleDebugWorkers flips capture of queue/scheduler worker events, the TUI
// equivalent of the dashboard's "Show worker queries" toggle. devtoolsops
// writes the sentinel and config flag; the next render reads the new state.
func (m *Model) toggleDebugWorkers() tea.Cmd {
	cfg, _ := config.LoadGlobal()
	enabled := cfg != nil && cfg.IsDevtoolsWorkers()
	if _, err := devtoolsops.SetWorkers(!enabled); err != nil {
		m.setStatus("worker capture: "+err.Error(), 4*time.Second)
		return nil
	}
	if enabled {
		m.setStatus("worker capture off", 3*time.Second)
	} else {
		m.setStatus("worker capture on", 3*time.Second)
	}
	return nil
}

func debugWorkersStateLabel() string {
	cfg, _ := config.LoadGlobal()
	if cfg != nil && cfg.IsDevtoolsWorkers() {
		return runningStyle.Render("workers on")
	}
	return dimStyle.Render("workers off (w)")
}

// debugMatches reports whether an event matches the search needle across its
// human-meaningful fields, including the raw Data JSON so a lens row's
// kind-specific values (sql, job class, mail subject…) are searchable.
func debugMatches(ev lerddumps.Event, needle string) bool {
	if strings.Contains(strings.ToLower(ev.Ctx.Site), needle) ||
		strings.Contains(strings.ToLower(ev.Ctx.Request), needle) ||
		strings.Contains(strings.ToLower(ev.Ctx.Worker), needle) ||
		strings.Contains(strings.ToLower(ev.Label), needle) ||
		strings.Contains(strings.ToLower(ev.Src.File), needle) ||
		strings.Contains(strings.ToLower(ev.Text), needle) {
		return true
	}
	return len(ev.Data) > 0 && strings.Contains(strings.ToLower(string(ev.Data)), needle)
}

// debugFiltered returns the active lens's events that pass the ctx chip and
// search filters, in buffer (arrival) order. A non-empty site restricts to
// that site's events (the per-site Debug tab); "" is the global D view.
func (m *Model) debugFiltered(site string) []lerddumps.Event {
	kind := m.activeLensKind()
	needle := strings.ToLower(strings.TrimSpace(m.dumpsFilter))
	out := make([]lerddumps.Event, 0, len(m.debug))
	for _, ev := range m.debug {
		if ev.Kind != kind {
			continue
		}
		if site != "" && ev.Ctx.Site != site {
			continue
		}
		if m.dumpsCtxFilter != "" && !strings.EqualFold(ev.Ctx.Type, m.dumpsCtxFilter) {
			continue
		}
		if needle != "" && !debugMatches(ev, needle) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

// debugVisibleEvents flattens the active lens into the exact top-to-bottom
// order the renderer walks, so the cursor index and enter-to-expand stay in
// lockstep with what's on screen. Dumps are newest-first flat; grouped lenses
// flatten their groups.
func (m *Model) debugVisibleEvents(site string) []lerddumps.Event {
	if m.activeLensKind() == lerddumps.KindDump {
		evs := m.debugFiltered(site)
		reverseEvents(evs)
		return evs
	}
	var out []lerddumps.Event
	for _, g := range m.debugGroups(site) {
		out = append(out, g.events...)
	}
	return out
}

// toggleDumpExpand flips expansion for the cursor's event in the active lens.
// The next render reads the new state.
func (m *Model) toggleDumpExpand() tea.Cmd {
	vis := m.debugVisibleEvents("")
	if m.dumpsCursor < 0 || m.dumpsCursor >= len(vis) {
		return nil
	}
	if m.dumpsExpanded == nil {
		m.dumpsExpanded = map[string]bool{}
	}
	id := vis[m.dumpsCursor].ID
	m.dumpsExpanded[id] = !m.dumpsExpanded[id]
	return nil
}

// debugGroup is one request's worth of events for a grouped lens.
type debugGroup struct {
	label, ts, worker string
	events            []lerddumps.Event // newest first
	nPlusOne          bool
}

func debugGroupKey(ev lerddumps.Event) string {
	if ev.Ctx.RID != "" {
		return "rid:" + ev.Ctx.RID
	}
	if ev.Ctx.Type == "fpm" {
		return fmt.Sprintf("fpm:%s:%s:%d", ev.Ctx.Site, ev.Ctx.Request, ev.Ctx.PID)
	}
	var bucket int64
	if t, err := time.Parse(time.RFC3339Nano, ev.TS); err == nil {
		bucket = t.Unix() / 5
	}
	return fmt.Sprintf("cli:%s:%d:%d", ev.Ctx.Site, ev.Ctx.PID, bucket)
}

func debugGroupLabel(ev lerddumps.Event) string {
	prefix := ""
	if ev.Ctx.Site != "" {
		prefix = "[" + ev.Ctx.Site + "] "
	}
	if ev.Ctx.Worker != "" {
		return prefix + ev.Ctx.Worker
	}
	if ev.Ctx.Type == "fpm" {
		req := ev.Ctx.Request
		if req == "" {
			req = "(request)"
		}
		return prefix + req
	}
	return fmt.Sprintf("%scli (pid %d)", prefix, ev.Ctx.PID)
}

// debugGroups buckets the active lens's filtered events by request, newest
// group first and newest event first within each group, matching the web
// Debug window. N+1 is flagged for the query lens.
func (m *Model) debugGroups(site string) []debugGroup {
	idx := map[string]int{}
	var groups []debugGroup
	for _, ev := range m.debugFiltered(site) {
		key := debugGroupKey(ev)
		i, ok := idx[key]
		if !ok {
			idx[key] = len(groups)
			i = len(groups)
			groups = append(groups, debugGroup{label: debugGroupLabel(ev), worker: ev.Ctx.Worker, ts: ev.TS})
		}
		groups[i].events = append(groups[i].events, ev)
		if ev.TS > groups[i].ts {
			groups[i].ts = ev.TS
		}
	}
	queryLens := m.activeLensKind() == lerddumps.KindQuery
	for i := range groups {
		reverseEvents(groups[i].events)
		if queryLens {
			groups[i].nPlusOne = queryGroupNPlusOne(groups[i].events)
		}
	}
	sort.SliceStable(groups, func(a, b int) bool { return groups[a].ts > groups[b].ts })
	return groups
}

func reverseEvents(s []lerddumps.Event) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// nPlusOneThresholdTUI mirrors the dashboard's NPLUSONE_AT so the badge agrees.
const nPlusOneThresholdTUI = 3

var (
	dbgReSQLSingle = regexp.MustCompile(`'(?:[^'\\]|\\.)*'`)
	dbgReSQLDouble = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	dbgReSQLNum    = regexp.MustCompile(`\b\d+\b`)
	dbgReSQLWS     = regexp.MustCompile(`\s+`)
)

// normalizeSQL collapses literals so structurally-identical queries share a
// fingerprint, mirroring stores/queries.ts and the N+1 notifier.
func normalizeSQL(sql string) string {
	sql = dbgReSQLSingle.ReplaceAllString(sql, "?")
	sql = dbgReSQLDouble.ReplaceAllString(sql, "?")
	sql = dbgReSQLNum.ReplaceAllString(sql, "?")
	sql = dbgReSQLWS.ReplaceAllString(sql, " ")
	return strings.ToLower(strings.TrimSpace(sql))
}

func queryGroupNPlusOne(evs []lerddumps.Event) bool {
	counts := map[string]int{}
	for _, ev := range evs {
		if q, ok := ev.Query(); ok {
			fp := normalizeSQL(q.SQL)
			counts[fp]++
			if counts[fp] >= nPlusOneThresholdTUI {
				return true
			}
		}
	}
	return false
}

// countKind counts buffered events of a kind, optionally scoped to one site.
func countKind(evs []lerddumps.Event, kind, site string) int {
	n := 0
	for _, ev := range evs {
		if ev.Kind == kind && (site == "" || ev.Ctx.Site == site) {
			n++
		}
	}
	return n
}

// renderDebugTabs draws the lens tab bar with per-lens buffered counts (scoped
// to site when non-empty); the active lens wears the key-chip background, the
// rest are dim.
func renderDebugTabs(m *Model, site string) string {
	counts := map[string]int{}
	for _, ev := range m.debug {
		if site == "" || ev.Ctx.Site == site {
			counts[ev.Kind]++
		}
	}
	parts := make([]string, 0, len(debugLenses))
	for i, l := range debugLenses {
		label := l.label
		if c := counts[l.kind]; c > 0 {
			label = fmt.Sprintf("%s %d", l.label, c)
		}
		if i == m.debugLens {
			parts = append(parts, keyChipStyle.Render(" "+label+" "))
		} else {
			parts = append(parts, dimStyle.Render(label))
		}
	}
	return strings.Join(parts, "  ")
}

// debugContentLines renders the whole Debug pane: the bridge/worker state, the
// lens tab bar, the help and filter rows, then the active lens body. Returns
// the absolute cursor line so the viewport keeps the selection on screen.
func debugContentLines(m *Model, focused bool, innerW int) ([]string, int) {
	out := make([]string, 0, len(m.debug)*3+12)
	add := func(s string) { out = append(out, padToWidth(clipLine(s, innerW), innerW)) }

	add(sectionStyle.Render("Debug") + "  " + dumpsBridgeStateLabel() + "  " + debugWorkersStateLabel())
	add("  " + renderDebugTabs(m, ""))
	add(dimStyle.Render("  [ ] lens · / search · 1/2 ctx · enter expand · w workers · c clear · T bridge · D return"))
	add("  " + renderDumpsChips(m.dumpsCtxFilter))
	if m.dumpsFilterActive || m.dumpsFilter != "" {
		add(padToWidth(filterBar(m.dumpsFilter, m.dumpsFilterActive), innerW))
	}
	add("")

	if m.activeLensKind() == lerddumps.KindDump {
		return appendDumpsLens(m, focused, innerW, out)
	}
	return appendGroupedLens(m, focused, innerW, out)
}

// appendDumpsLens renders the flat, newest-first dump list (unchanged UX).
func appendDumpsLens(m *Model, focused bool, innerW int, out []string) ([]string, int) {
	add := func(s string) { out = append(out, padToWidth(clipLine(s, innerW), innerW)) }
	vis := m.debugVisibleEvents("")
	buffered := countKind(m.debug, lerddumps.KindDump, "")
	add(dimStyle.Render(fmt.Sprintf("  %d shown / %d buffered (cap %d)", len(vis), buffered, dumpsBufferCap)))
	add("")

	if len(vis) == 0 {
		if buffered == 0 {
			add(dimStyle.Render("  no dumps yet"))
			add("")
			add("  " + dimStyle.Render("1. enable with ") + accentStyle.Render("T") + dimStyle.Render(" or ") + accentStyle.Render("lerd dump on"))
			add("  " + dimStyle.Render("2. trigger a ") + accentStyle.Render("dump()") + dimStyle.Render(" / ") + accentStyle.Render("dd()") + dimStyle.Render(" in your PHP code"))
		} else {
			add(dimStyle.Render("  no dumps match this filter"))
		}
		return out, 0
	}

	if m.dumpsCursor < 0 {
		m.dumpsCursor = 0
	}
	if m.dumpsCursor >= len(vis) {
		m.dumpsCursor = len(vis) - 1
	}
	cursorLine := len(out)
	for row, ev := range vis {
		entry := toDumpEntry(ev)
		marker := "  "
		if row == m.dumpsCursor {
			marker = "▶ "
			cursorLine = len(out)
		}
		hdr := marker + dumpHeaderLine(entry)
		if focused && row == m.dumpsCursor {
			hdr = selectedStyle.Render(hdr)
		}
		add(hdr)
		expanded := m.dumpsExpanded != nil && m.dumpsExpanded[entry.ID]
		for _, ln := range dumpBodyLines(entry, innerW-4, expanded) {
			add("    " + dimStyle.Render(ln))
		}
		add("")
	}
	return out, cursorLine
}

// appendGroupedLens renders queries/jobs/views/mail/cache/events/http grouped
// by request, with per-row detail on expand and N+1/slow flags for queries.
func appendGroupedLens(m *Model, focused bool, innerW int, out []string) ([]string, int) {
	add := func(s string) { out = append(out, padToWidth(clipLine(s, innerW), innerW)) }
	kind := m.activeLensKind()
	groups := m.debugGroups("")
	total := 0
	for _, g := range groups {
		total += len(g.events)
	}
	buffered := countKind(m.debug, kind, "")
	add(dimStyle.Render(fmt.Sprintf("  %d shown / %d buffered (cap %d)", total, buffered, dumpsBufferCap)))
	add("")

	if total == 0 {
		if buffered == 0 {
			add(dimStyle.Render("  no " + lensNoun(kind) + " captured yet"))
			add("")
			add("  " + dimStyle.Render("enable with ") + accentStyle.Render("T") + dimStyle.Render("; worker events also need ") + accentStyle.Render("w"))
		} else {
			add(dimStyle.Render("  no " + lensNoun(kind) + " match this filter"))
		}
		return out, 0
	}

	if m.dumpsCursor < 0 {
		m.dumpsCursor = 0
	}
	if m.dumpsCursor >= total {
		m.dumpsCursor = total - 1
	}

	cursorLine := len(out)
	row := 0
	for _, g := range groups {
		meta := fmt.Sprintf("  %s · %d", shortTime(g.ts), len(g.events))
		if g.worker != "" {
			meta = "  worker" + meta
		}
		head := "  " + accentStyle.Render(g.label) + dimStyle.Render(meta)
		if g.nPlusOne {
			head += "  " + failingStyle.Render("N+1")
		}
		add(head)

		var dup map[string]int
		if kind == lerddumps.KindQuery {
			dup = map[string]int{}
			for _, ev := range g.events {
				if q, ok := ev.Query(); ok {
					dup[normalizeSQL(q.SQL)]++
				}
			}
		}

		for _, ev := range g.events {
			marker := "  "
			if row == m.dumpsCursor {
				marker = "▶ "
				cursorLine = len(out)
			}
			main := marker + debugRowMain(kind, ev, dup)
			if focused && row == m.dumpsCursor {
				main = selectedStyle.Render(main)
			}
			add(main)
			if m.dumpsExpanded != nil && m.dumpsExpanded[ev.ID] {
				for _, ln := range debugRowDetail(kind, ev) {
					add("      " + dimStyle.Render(ln))
				}
			}
			row++
		}
		add("")
	}
	return out, cursorLine
}

func lensNoun(kind string) string {
	switch kind {
	case lerddumps.KindQuery:
		return "queries"
	case lerddumps.KindJob:
		return "jobs"
	case lerddumps.KindView:
		return "views"
	case lerddumps.KindMail:
		return "mail"
	case lerddumps.KindCache:
		return "cache events"
	case lerddumps.KindEvent:
		return "events"
	case lerddumps.KindHTTP:
		return "HTTP calls"
	default:
		return "events"
	}
}

// Light decoders for the kind-specific Data payloads the lerd_devtools
// adapters emit; only the fields the lens rows render are pulled out.
type jobData struct {
	Class      string `json:"class"`
	Status     string `json:"status"`
	Connection string `json:"connection"`
	Exception  string `json:"exception"`
}

type viewData struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	DataKeys []string `json:"data_keys"`
}

type mailData struct {
	Subject string   `json:"subject"`
	To      []string `json:"to"`
	From    []string `json:"from"`
	Cc      []string `json:"cc"`
}

type cacheData struct {
	Key   string `json:"key"`
	Op    string `json:"op"`
	Store string `json:"store"`
}

type httpData struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Status int    `json:"status"`
	Failed bool   `json:"failed"`
}

type namedData struct {
	Name string `json:"name"`
}

// debugRowMain returns the one-line summary for an event in the given lens.
// dup is the per-group fingerprint counts (query lens only) for the ×N badge.
func debugRowMain(kind string, ev lerddumps.Event, dup map[string]int) string {
	switch kind {
	case lerddumps.KindQuery:
		q, _ := ev.Query()
		line := oneLine(q.SQL) + "  " + dimStyle.Render(fmtMS(q.TimeMS)+"ms")
		if q.TimeMS >= 100 {
			line += " " + failingStyle.Render("slow")
		}
		if dup != nil {
			if n := dup[normalizeSQL(q.SQL)]; n >= 2 {
				line += " " + accentStyle.Render(fmt.Sprintf("×%d", n))
			}
		}
		return line
	case lerddumps.KindJob:
		var d jobData
		_ = json.Unmarshal(ev.Data, &d)
		return d.Class + "  " + statusTag(d.Status)
	case lerddumps.KindView:
		var d viewData
		_ = json.Unmarshal(ev.Data, &d)
		return d.Name
	case lerddumps.KindMail:
		var d mailData
		_ = json.Unmarshal(ev.Data, &d)
		subject := d.Subject
		if subject == "" {
			subject = "(no subject)"
		}
		if len(d.To) > 0 {
			subject += dimStyle.Render("  → " + d.To[0])
		}
		return subject
	case lerddumps.KindCache:
		var d cacheData
		_ = json.Unmarshal(ev.Data, &d)
		return d.Key + "  " + statusTag(d.Op)
	case lerddumps.KindHTTP:
		var d httpData
		_ = json.Unmarshal(ev.Data, &d)
		line := d.Method + " " + oneLine(d.URL)
		switch {
		case d.Status > 0:
			line += "  " + httpStatusTag(d.Status)
		case d.Failed:
			line += "  " + failingStyle.Render("failed")
		default:
			line += "  " + dimStyle.Render("sent")
		}
		return line
	default: // events
		var d namedData
		_ = json.Unmarshal(ev.Data, &d)
		return d.Name
	}
}

// debugRowDetail returns the expanded detail lines for an event in the lens.
func debugRowDetail(kind string, ev lerddumps.Event) []string {
	var out []string
	switch kind {
	case lerddumps.KindQuery:
		q, _ := ev.Query()
		if len(q.Bindings) > 0 {
			out = append(out, "bindings: "+oneLine(fmt.Sprint(q.Bindings)))
		}
		if q.Connection != "" {
			conn := q.Connection
			if q.RWType != "" {
				conn += " (" + q.RWType + ")"
			}
			out = append(out, conn)
		}
		out = appendCaller(out, ev)
	case lerddumps.KindJob:
		var d jobData
		_ = json.Unmarshal(ev.Data, &d)
		if d.Connection != "" {
			out = append(out, "connection: "+d.Connection)
		}
		if d.Exception != "" {
			out = append(out, "exception: "+oneLine(d.Exception))
		}
	case lerddumps.KindView:
		var d viewData
		_ = json.Unmarshal(ev.Data, &d)
		if d.Path != "" {
			out = append(out, "template: "+shortPath(d.Path))
		}
		if len(d.DataKeys) > 0 {
			out = append(out, "data: "+strings.Join(d.DataKeys, ", "))
		}
	case lerddumps.KindMail:
		var d mailData
		_ = json.Unmarshal(ev.Data, &d)
		line := ""
		if len(d.From) > 0 {
			line += "from " + strings.Join(d.From, ", ") + " · "
		}
		line += "to " + strings.Join(d.To, ", ")
		if len(d.Cc) > 0 {
			line += " · cc " + strings.Join(d.Cc, ", ")
		}
		out = append(out, line)
	case lerddumps.KindCache:
		var d cacheData
		_ = json.Unmarshal(ev.Data, &d)
		if d.Store != "" {
			out = append(out, "store: "+d.Store)
		}
	default: // http, events
		out = appendCaller(out, ev)
	}
	return out
}

func appendCaller(out []string, ev lerddumps.Event) []string {
	if ev.Src.File != "" {
		out = append(out, fmt.Sprintf("%s:%d", shortPath(ev.Src.File), ev.Src.Line))
	}
	return out
}

func statusTag(v string) string {
	switch v {
	case "processed", "hit":
		return runningStyle.Render(v)
	case "failed":
		return failingStyle.Render(v)
	case "":
		return ""
	default:
		return dimStyle.Render(v)
	}
}

func httpStatusTag(code int) string {
	s := fmt.Sprintf("%d", code)
	switch {
	case code >= 500:
		return failingStyle.Render(s)
	case code >= 400:
		return accentStyle.Render(s)
	case code >= 200 && code < 300:
		return runningStyle.Render(s)
	default:
		return dimStyle.Render(s)
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func fmtMS(n float64) string {
	if n < 10 {
		return fmt.Sprintf("%.2f", n)
	}
	return fmt.Sprintf("%.1f", n)
}
