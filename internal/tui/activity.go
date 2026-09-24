package tui

import (
	"fmt"
	"time"

	"github.com/geodro/lerd/internal/siteinfo"
)

// activityCap bounds the recent-activity ring so a long-running TUI doesn't
// accumulate events forever. Matches the web UI's MAX of 30.
const activityCap = 30

type activityTone int

const (
	toneGood activityTone = iota
	toneBad
	toneWarn
)

// activityEvent is one entry in the Recent Activity feed, derived by diffing
// two consecutive snapshots. text is the pre-built label; at is used to render
// a relative timestamp lazily so the line re-renders without per-event timers.
type activityEvent struct {
	text string
	tone activityTone
	at   time.Time
}

func (e activityEvent) render() string {
	dot := runningStyle.Render(glyphRunning)
	switch e.tone {
	case toneBad:
		dot = failingStyle.Render(glyphFailing)
	case toneWarn:
		dot = pausedStyle.Render(glyphPaused)
	}
	return dot + " " + e.text + "  " + dimStyle.Render(humanAgo(time.Since(e.at)))
}

// humanAgo renders a coarse relative duration ("now", "5m", "2h", "3d").
func humanAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// recordActivity diffs the incoming snapshot against the previous one, prepends
// any new events to the ring (newest first), and stores the snapshot as the
// baseline for next time. The first snapshot only sets the baseline.
func (m *Model) recordActivity(next Snapshot, now time.Time) {
	if m.prevSnap != nil {
		if evs := diffSnapshots(*m.prevSnap, next, now); len(evs) > 0 {
			m.activity = append(evs, m.activity...)
			if len(m.activity) > activityCap {
				m.activity = m.activity[:activityCap]
			}
		}
	}
	cp := next
	m.prevSnap = &cp
}

// diffSnapshots compares two snapshots and returns the state-change events
// between them, mirroring the subset of the web UI's activity diff the TUI
// can derive from its own snapshot: site link/remove/pause/resume/run/stop,
// service add/remove/start/stop, worker fail/heal, and DNS transitions.
func diffSnapshots(prev, cur Snapshot, now time.Time) []activityEvent {
	var out []activityEvent
	add := func(text string, tone activityTone) {
		out = append(out, activityEvent{text: text, tone: tone, at: now})
	}

	siteSubject := func(s siteinfo.EnrichedSite) string {
		if d := s.PrimaryDomain(); d != "" {
			return d
		}
		return s.Name
	}

	prevSites := make(map[string]siteinfo.EnrichedSite, len(prev.Sites))
	for _, s := range prev.Sites {
		prevSites[s.Name] = s
	}
	curSites := make(map[string]bool, len(cur.Sites))
	for _, s := range cur.Sites {
		curSites[s.Name] = true
		old, ok := prevSites[s.Name]
		if !ok {
			if !cur.Private[s.Name] {
				add("linked "+siteSubject(s), toneGood)
			}
			continue
		}
		switch {
		case old.Paused != s.Paused:
			if s.Paused {
				add("paused "+siteSubject(s), toneWarn)
			} else {
				add("resumed "+siteSubject(s), toneGood)
			}
		case old.FPMRunning != s.FPMRunning:
			if s.FPMRunning {
				add(siteSubject(s)+" started", toneGood)
			} else {
				add(siteSubject(s)+" stopped", toneBad)
			}
		}
	}
	for name, s := range prevSites {
		// Streaming mode drops a private site from the snapshot, and naming it
		// here would put it straight back on the shared screen.
		if !curSites[name] && !prev.Private[name] {
			add("removed "+siteSubject(s), toneBad)
		}
	}

	prevSvc := make(map[string]ServiceRow)
	for _, s := range prev.Services {
		if s.WorkerKind == "" {
			prevSvc[s.Name] = s
		}
	}
	curSvc := make(map[string]bool)
	for _, s := range cur.Services {
		if s.WorkerKind != "" {
			continue
		}
		curSvc[s.Name] = true
		old, ok := prevSvc[s.Name]
		if !ok {
			add("added "+s.Name, toneGood)
			continue
		}
		if (old.State == stateRunning) != (s.State == stateRunning) {
			if s.State == stateRunning {
				add(s.Name+" started", toneGood)
			} else {
				add(s.Name+" stopped", toneBad)
			}
		}
	}
	for name := range prevSvc {
		if !curSvc[name] {
			add("removed "+name, toneBad)
		}
	}

	prevFail := make(map[string]bool)
	for _, n := range failingWorkerNames(prev) {
		prevFail[n] = true
	}
	curFail := make(map[string]bool)
	for _, n := range failingWorkerNames(cur) {
		curFail[n] = true
		if !prevFail[n] {
			add(n+" failed", toneBad)
		}
	}
	for n := range prevFail {
		if !curFail[n] {
			add(n+" healed", toneGood)
		}
	}

	if prev.Status.DNSOk != cur.Status.DNSOk || prev.Status.DNSDegraded != cur.Status.DNSDegraded {
		switch {
		case cur.Status.DNSOk:
			add("DNS recovered", toneGood)
		case cur.Status.DNSDegraded:
			add("DNS degraded", toneWarn)
		default:
			add("DNS down", toneBad)
		}
	}

	return out
}
