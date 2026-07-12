package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	lerddumps "github.com/geodro/lerd/internal/dumps"
	"github.com/geodro/lerd/internal/siteinfo"
	zone "github.com/lrstanley/bubblezone"
)

// siteTab identifies which sub-view of Site detail is showing. Tabs let the
// detail pane mirror the web UI's Overview / Logs / Env / Dumps split without
// forcing users to scroll past the toggles every time they want to inspect a
// different facet of the site.
type siteTab int

const (
	tabSiteOverview siteTab = iota
	tabSiteLogs
	tabSiteEnv
	tabSiteDebug
	tabSiteDoctor
)

// envReadLimit caps how much of a site's .env we slurp. 256 KB is two
// orders of magnitude above a realistic .env and small enough that a
// pathological file never wedges the render loop.
const envReadLimit = 256 * 1024

// siteTabLabel returns the title shown in the tab strip header.
func siteTabLabel(t siteTab) string {
	switch t {
	case tabSiteLogs:
		return "Logs"
	case tabSiteEnv:
		return "Env"
	case tabSiteDebug:
		return "Debug"
	case tabSiteDoctor:
		return "Doctor"
	default:
		return "Overview"
	}
}

// siteTabsHeader renders the tab strip across the top of the site detail
// pane, e.g. "[1] Overview  [2] Logs  [3] Env". The active tab is highlighted
// in the accent colour; the others are dimmed. Lives at the head of every site
// detail variant so the user always sees the shortcuts and which tab is active
// without scrolling.
func siteTabsHeader(active siteTab, tabs []siteTab) string {
	parts := make([]string, 0, len(tabs))
	for i, t := range tabs {
		label := fmt.Sprintf("[%d] %s", i+1, siteTabLabel(t))
		if t == active {
			label = selectedStyle.Render(label)
		} else {
			label = dimStyle.Render(label)
		}
		// Each tab label is clickable; handleMouse maps the zone to selectSiteTab.
		parts = append(parts, zone.Mark(fmt.Sprintf("sitetab:%d", i), label))
	}
	return "  " + strings.Join(parts, "  ")
}

// renderSiteTabHeader returns the two-line block that precedes every site
// tab's content: the tab strip and a divider. Centralised so each tab
// renderer pads to the same width and the user sees a consistent header.
func renderSiteTabHeader(active siteTab, innerW int, tabs []siteTab) []string {
	return []string{
		padToWidth(clipLine(siteTabsHeader(active, tabs), innerW), innerW),
		"",
	}
}

// availableSiteTabs returns the tabs a site offers, in display order. The doctor
// runs framework-agnostic checks, so every site gets the tab. This is the single
// source the strip numbering, the number-key shortcuts, and the render dispatch
// all derive from, so a tab's position, label, and availability can never drift.
func availableSiteTabs(s *siteinfo.EnrichedSite) []siteTab {
	tabs := []siteTab{tabSiteOverview, tabSiteLogs, tabSiteEnv, tabSiteDebug}
	if s != nil {
		tabs = append(tabs, tabSiteDoctor)
	}
	return tabs
}

// siteEnvContentLines reads the site's .env file and renders one line per
// row. Read-only (matches the web UI's SiteEnvTab in PR1; an editor lands
// in a later phase). Empty .env or missing file renders a helpful empty-
// state so users understand the file isn't on disk yet.
func siteEnvContentLines(m *Model, site *siteinfo.EnrichedSite, innerW int) []string {
	out := make([]string, 0, 32)
	out = append(out, renderSiteTabHeader(tabSiteEnv, innerW, availableSiteTabs(site))...)
	add := func(s string) { out = append(out, padToWidth(clipLine(s, innerW), innerW)) }

	if site == nil {
		add(dimStyle.Render("  no site selected"))
		return out
	}

	envPath := filepath.Join(site.Path, ".env")
	add(sectionStyle.Render(".env") + "  " + dimStyle.Render(envPath))
	add("")

	data, err := readBoundedFile(envPath, envReadLimit)
	switch {
	case os.IsNotExist(err):
		add(dimStyle.Render("  no .env on disk yet"))
		return out
	case err != nil:
		add(failingStyle.Render("  read error: ") + err.Error())
		return out
	case len(data) == 0:
		add(dimStyle.Render("  .env is empty"))
		return out
	}

	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		add("  " + line)
	}
	return out
}

// siteDebugContentLines renders the focused site's slice of the Debug window:
// the active lens (Dumps · Queries · Jobs · Views · Mail · Cache · Events ·
// HTTP) scoped to this site, read-only. `[` / `]` switch lens; the global ctx
// chips and search needle set on the D view still apply. Rows are shown with
// their detail inline (no cursor/expand on a site scroll surface), so the tab
// is a per-site debug feed, not just dumps.
func siteDebugContentLines(m *Model, site *siteinfo.EnrichedSite, innerW int) []string {
	out := make([]string, 0, 32)
	out = append(out, renderSiteTabHeader(tabSiteDebug, innerW, availableSiteTabs(site))...)
	add := func(s string) { out = append(out, padToWidth(clipLine(s, innerW), innerW)) }

	if site == nil {
		add(dimStyle.Render("  no site selected"))
		return out
	}

	kind := m.activeLensKind()
	add(sectionStyle.Render("Debug for "+site.Name) + "  " + dumpsBridgeStateLabel())
	add("")
	add("  " + renderDebugTabs(m, site.Name))
	add("")
	hint := "  [ ] switch lens · D for the full window"
	if m.dumpsCtxFilter != "" {
		hint += " · ctx:" + m.dumpsCtxFilter
	}
	if needle := strings.TrimSpace(m.dumpsFilter); needle != "" {
		hint += " · /" + needle
	}
	add(dimStyle.Render(hint))
	add("")

	buffered := countKind(m.debug, kind, site.Name)

	if kind == lerddumps.KindDump {
		vis := m.debugVisibleEvents(site.Name) // newest-first dump events
		add(dimStyle.Render(fmt.Sprintf("  %d shown / %d buffered", len(vis), buffered)))
		add("")
		if len(vis) == 0 {
			add(dimStyle.Render("  no dumps from this site yet"))
			add("")
			add("  " + dimStyle.Render("press ") + accentStyle.Render("D") + dimStyle.Render(" then ") + accentStyle.Render("T") + dimStyle.Render(" to enable the bridge; this site's dumps land here"))
			return out
		}
		for _, ev := range vis {
			e := toDumpEntry(ev)
			add(dumpHeaderLine(e))
			for _, ln := range dumpPreviewLines(e, innerW-4) {
				add("    " + dimStyle.Render(ln))
			}
			add("")
		}
		return out
	}

	groups := m.debugGroups(site.Name)
	total := 0
	for _, g := range groups {
		total += len(g.events)
	}
	add(dimStyle.Render(fmt.Sprintf("  %d shown / %d buffered", total, buffered)))
	add("")
	if total == 0 {
		add(dimStyle.Render("  no " + lensNoun(kind) + " from this site yet"))
		add("")
		add("  " + dimStyle.Render("press ") + accentStyle.Render("D") + dimStyle.Render(" then ") + accentStyle.Render("T") + dimStyle.Render(" to enable capture; worker events also need ") + accentStyle.Render("w"))
		return out
	}
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
			add("  " + debugRowMain(kind, ev, dup))
			for _, ln := range debugRowDetail(kind, ev) {
				add("      " + dimStyle.Render(ln))
			}
		}
		add("")
	}
	return out
}

// siteLogsActive reports whether the Sites tab is showing the Logs tab for a
// selected site, in which case the detail column is given over to the streaming
// tail rather than the usual rows.
func (m *Model) siteLogsActive() bool {
	return m.activeTab == tabSites && m.detailMode == detailSite &&
		m.siteTab == tabSiteLogs && m.currentSite() != nil
}

// serviceLogsActive reports whether the Services tab should show a logs
// sub-pane beneath the service detail: any time a service or worker row is
// selected on that tab. The streaming tail is fed by the same logTail the
// manual `l` pane uses, retargeted by syncLogs as the selection moves.
func (m *Model) serviceLogsActive() bool {
	return m.activeTab == tabServices && m.currentService() != nil
}

// logsInDetail reports whether the tail is already showing inside the detail
// column, on either tab. The full-width `l` pane consults this so the same logs
// can't be drawn twice, and syncLogs so it keeps retargeting the tail as the
// selection moves even when that pane is closed.
func (m *Model) logsInDetail() bool {
	return m.siteLogsActive() || m.serviceLogsActive()
}

// readBoundedFile reads up to max bytes of path. Used for the env reader so
// a runaway file (cat /dev/zero > .env) can't OOM the TUI.
func readBoundedFile(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, max)
	n, err := f.Read(buf)
	if err != nil && err.Error() != "EOF" {
		return buf[:n], err
	}
	return buf[:n], nil
}

// openInBrowserCmd opens the focused row in the browser: a service's dashboard
// URL when the Services pane is focused, otherwise the focused site's primary
// domain. Falls back to a status-bar message when there's nothing to open or
// the platform lacks a known opener.
func (m *Model) openInBrowserCmd() tea.Cmd {
	switch m.activeTab {
	case tabServices:
		return m.openServiceDashboardCmd()
	case tabSites:
		site := m.currentSite()
		if site == nil {
			return nil
		}
		domain := site.PrimaryDomain()
		if domain == "" {
			m.setStatus("no domain to open for "+site.Name, 3*time.Second)
			return nil
		}
		scheme := "http"
		if site.Secured {
			scheme = "https"
		}
		return m.openURL(scheme + "://" + domain)
	}
	return nil
}

// openServiceDashboardCmd opens the focused service's dashboard URL. Worker
// rows and services without a dashboard get a status-bar note rather than a
// silent no-op, so the user knows the key was heard.
func (m *Model) openServiceDashboardCmd() tea.Cmd {
	svc := m.currentService()
	if svc == nil {
		return nil
	}
	if svc.Dashboard == "" {
		m.setStatus(svc.Name+" has no dashboard to open", 3*time.Second)
		return nil
	}
	return m.openURL(svc.Dashboard)
}

// openURL launches the default browser on url via the platform opener, or
// surfaces a status message when no opener exists. The browser detaches, so the
// command returns as soon as the opener is spawned.
func (m *Model) openURL(url string) tea.Cmd {
	opener := browserOpener()
	if opener == "" {
		m.setStatus("no browser opener available on "+runtime.GOOS, 3*time.Second)
		return nil
	}
	m.setStatus("opening "+url+"…", 3*time.Second)
	return func() tea.Msg {
		cmd := exec.Command(opener, url)
		runErr := cmd.Start()
		return ActionResult{Summary: "open " + url, Err: runErr}
	}
}

// browserOpener picks the platform command that launches the default
// browser. Linux uses xdg-open, macOS uses open. Returns "" on platforms
// where neither is appropriate so the caller surfaces a status message
// instead of erroring.
func browserOpener() string {
	switch runtime.GOOS {
	case "darwin":
		return "open"
	case "linux":
		return "xdg-open"
	default:
		return ""
	}
}
