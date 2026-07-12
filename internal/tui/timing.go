package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/reqstats"
	"github.com/geodro/lerd/internal/siteinfo"
)

// timingRanges is the window ladder `[` and `]` cycle, matching the web UI's
// range picker. An hour is the default: wide enough to cover a stretch of work
// without burying the request from a minute ago.
var timingRanges = []struct {
	label string
	dur   time.Duration
}{
	{"15m", 15 * time.Minute},
	{"1h", time.Hour},
	{"24h", 24 * time.Hour},
	{"7d", 7 * 24 * time.Hour},
}

const (
	defaultTimingRange = 1 // 1h
	timingRefresh      = 5 * time.Second
	timingRecentLimit  = 6
	timingRouteLimit   = 5
	timingBarWidth     = 18
)

// timingScope is one entry in the branch cycle. key is the reqstats identity the
// watcher records under, so a worktree reads its own traffic rather than having
// it folded into the parent site's.
type timingScope struct {
	label string
	key   string
}

// timingScopes returns the branch cycle for a site: the checkout itself, then
// one entry per git worktree.
func timingScopes(s *siteinfo.EnrichedSite) []timingScope {
	if s == nil {
		return nil
	}
	label := s.Branch
	if label == "" {
		label = "main"
	}
	out := []timingScope{{label: label, key: reqstats.Key(s.Name, "")}}
	for _, wt := range s.Worktrees {
		out = append(out, timingScope{label: wt.Branch, key: reqstats.Key(s.Name, wt.Branch)})
	}
	return out
}

// timingResultMsg carries a finished store read back into the model. cacheKey
// identifies the site, branch and window it was read for, so a result that lands
// after the user has moved on is discarded rather than shown against the wrong
// scope.
type timingResultMsg struct {
	cacheKey string
	analytic reqstats.Analytics
	recent   []reqstats.Record
	err      error
}

// timingCmd reads the durable request store off the main loop. The store is
// SQLite on the watcher's WAL file, so this is a cheap read, but it's still I/O
// and must never run inline in Update.
func timingCmd(cacheKey, key string, dur time.Duration) tea.Cmd {
	return func() tea.Msg {
		store, err := reqstats.OpenShared(config.RequestStatsDB())
		if err != nil {
			return timingResultMsg{cacheKey: cacheKey, err: err}
		}
		until := time.Now()
		a, err := store.SiteAnalytics(key, until.Add(-dur), until)
		if err != nil {
			return timingResultMsg{cacheKey: cacheKey, err: err}
		}
		recent, _ := store.Recent(key, timingRecentLimit)
		return timingResultMsg{cacheKey: cacheKey, analytic: a, recent: recent}
	}
}

// timingActive reports whether the request-timing panel is on screen: the site
// Overview, which is the only place it renders.
func (m *Model) timingActive() bool {
	return m.activeTab == tabSites && m.detailMode == detailSite &&
		m.siteTab == tabSiteOverview && m.currentSite() != nil
}

// currentTimingScope returns the focused branch scope, clamping the cycle index
// against the focused site's worktrees, which differ from site to site.
func (m *Model) currentTimingScope() (timingScope, bool) {
	scopes := timingScopes(m.currentSite())
	if len(scopes) == 0 {
		return timingScope{}, false
	}
	if m.timingScope >= len(scopes) {
		m.timingScope = 0
	}
	return scopes[m.timingScope], true
}

// timingCacheKey identifies the figures currently held: which site, which branch,
// over which window. A change to any of the three invalidates the cache.
func (m *Model) timingCacheKey() string {
	scope, ok := m.currentTimingScope()
	if !ok {
		return ""
	}
	return scope.key + "@" + timingRanges[m.timingRange].label
}

// ensureTiming loads the panel when the figures on hand are for a different
// site, branch or window, or have gone stale. A no-op otherwise, so it's safe to
// call on every tick and every navigation.
func (m *Model) ensureTiming() tea.Cmd {
	if !m.timingActive() {
		return nil
	}
	want := m.timingCacheKey()
	if want == "" {
		return nil
	}
	if want == m.timingKey && time.Since(m.timingAt) < timingRefresh {
		return nil
	}
	if want != m.timingKey {
		// Another scope entirely: drop the figures so the panel reads as loading
		// rather than showing one branch's numbers under another's heading.
		m.timingLoaded = false
	}
	scope, _ := m.currentTimingScope()
	m.timingKey = want
	m.timingAt = time.Now()
	return timingCmd(want, scope.key, timingRanges[m.timingRange].dur)
}

// cycleTimingRange steps the window, and cycleTimingScope the branch. Both drop
// the cached figures so the next ensureTiming reloads rather than leaving the
// previous scope's numbers under the new heading.
func (m *Model) cycleTimingRange(delta int) tea.Cmd {
	n := len(timingRanges)
	m.timingRange = ((m.timingRange+delta)%n + n) % n
	return m.ensureTiming()
}

func (m *Model) cycleTimingScope(delta int) tea.Cmd {
	scopes := timingScopes(m.currentSite())
	if len(scopes) <= 1 {
		return nil
	}
	n := len(scopes)
	m.timingScope = ((m.timingScope+delta)%n + n) % n
	return m.ensureTiming()
}

// timingSectionLines renders the request-timing panel appended to the site
// Overview: the headline percentiles, the status mix, the response-time
// distribution, the slowest routes, and the tail of recent requests.
func timingSectionLines(m *Model, site *siteinfo.EnrichedSite, innerW int) []string {
	var out []string
	add := func(s string) { out = append(out, padToWidth(clipLine(s, innerW), innerW)) }

	scope, ok := m.currentTimingScope()
	if !ok {
		return nil
	}
	rng := timingRanges[m.timingRange].label

	head := sectionStyle.Render("Request timing")
	head += "  " + accentStyle.Render(rng)
	if len(timingScopes(site)) > 1 {
		head += dimStyle.Render(" · ") + scope.label
	}
	head += "  " + dimStyle.Render("[ ] window · b branch")
	add(head)

	switch {
	case m.timingErr != nil:
		add(dimStyle.Render("  timing unavailable: start the watcher to record requests"))
		return out
	case !m.timingLoaded:
		add(dimStyle.Render("  reading…"))
		return out
	case m.timing.Samples == 0:
		add(dimStyle.Render("  no requests in the last " + rng))
		return out
	}

	a := m.timing
	line := fmt.Sprintf("  p50 %s · p95 %s · %d requests", ms(a.MedianMillis), ms(a.P95Millis), a.Samples)
	if a.ColdStarts > 0 {
		line += fmt.Sprintf(" · %d cold", a.ColdStarts)
	}
	add(line)
	add("  " + statusMix(a.Status))
	add("")

	add("  " + sectionStyle.Render("Response times"))
	for _, row := range distributionRows(a.Distribution) {
		add(row)
	}
	add("")

	if routes := topRoutes(a.Routes); len(routes) > 0 {
		add("  " + sectionStyle.Render("Slowest routes"))
		for _, r := range routes {
			// Route already carries the method ("GET /products/:id"), so it isn't
			// repeated as its own column.
			add(fmt.Sprintf("    %-44s %8s  %4d×",
				clipLine(r.Route, 44), ms(r.RecentP95Millis), r.Samples))
		}
		add("")
	}

	if len(m.timingRecent) > 0 {
		add("  " + sectionStyle.Render("Recent"))
		for _, r := range m.timingRecent {
			uri := r.URI
			if uri == "" {
				uri = r.Route
			}
			add(fmt.Sprintf("    %s  %-6s %s %-30s %8s",
				r.At.Format("15:04:05"), r.Method, statusGlyph(r.Status), clipLine(uri, 30), ms(r.Millis)))
		}
	}
	return out
}

// distributionRows renders the latency histogram as one bar per bucket, scaled
// against the busiest bucket so the shape reads at any request volume.
func distributionRows(buckets []reqstats.LatencyBucket) []string {
	peak := 0
	for _, b := range buckets {
		if b.Count > peak {
			peak = b.Count
		}
	}
	if peak == 0 {
		return nil
	}
	rows := make([]string, 0, len(buckets))
	for _, b := range buckets {
		label := "≥1s"
		if b.UpperMillis > 0 {
			label = bucketLabel(b.UpperMillis)
		}
		width := b.Count * timingBarWidth / peak
		bar := strings.Repeat("▇", width)
		if b.Count > 0 && width == 0 {
			bar = "▏" // a bucket with traffic never renders as empty
		}
		rows = append(rows, fmt.Sprintf("    %-8s %-*s %4d", label, timingBarWidth, bar, b.Count))
	}
	return rows
}

// topRoutes ranks by recent p95, the same recency-aware figure the web UI sorts
// on, so a route that's been fixed drops off as newer, faster samples arrive.
func topRoutes(routes []reqstats.RouteStat) []reqstats.RouteStat {
	out := append([]reqstats.RouteStat(nil), routes...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].RecentP95Millis != out[j].RecentP95Millis {
			return out[i].RecentP95Millis > out[j].RecentP95Millis
		}
		return out[i].Route < out[j].Route
	})
	if len(out) > timingRouteLimit {
		out = out[:timingRouteLimit]
	}
	return out
}

// statusMix renders the response-status breakdown, colouring only the classes
// that actually occurred so a clean window stays quiet.
func statusMix(s reqstats.StatusCounts) string {
	parts := []string{}
	if s.C2xx > 0 {
		parts = append(parts, runningStyle.Render(fmt.Sprintf("2xx %d", s.C2xx)))
	}
	if s.C3xx > 0 {
		parts = append(parts, dimStyle.Render(fmt.Sprintf("3xx %d", s.C3xx)))
	}
	if s.C4xx > 0 {
		parts = append(parts, pausedStyle.Render(fmt.Sprintf("4xx %d", s.C4xx)))
	}
	if s.C5xx > 0 {
		parts = append(parts, failingStyle.Render(fmt.Sprintf("5xx %d", s.C5xx)))
	}
	return strings.Join(parts, dimStyle.Render(" · "))
}

// statusGlyph colours a status code by class for the recent-requests list.
func statusGlyph(status int) string {
	text := fmt.Sprintf("%3d", status)
	switch status / 100 {
	case 2:
		return runningStyle.Render(text)
	case 4:
		return pausedStyle.Render(text)
	case 5:
		return failingStyle.Render(text)
	}
	return dimStyle.Render(text)
}

// bucketLabel names a distribution bucket by its upper bound. The edges are all
// whole milliseconds, so they carry no decimal.
func bucketLabel(upper float64) string {
	if upper >= 1000 {
		return fmt.Sprintf("<%.0fs", upper/1000)
	}
	return fmt.Sprintf("<%.0fms", upper)
}

// ms formats a millisecond figure compactly: sub-second stays in ms, anything
// slower reads in seconds, where the extra precision is noise.
func ms(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.2fs", v/1000)
	}
	if v >= 100 {
		return fmt.Sprintf("%.0fms", v)
	}
	return fmt.Sprintf("%.1fms", v)
}
