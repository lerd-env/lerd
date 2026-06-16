package ui

import (
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/idle"
)

// wtKey is the idle/activity key for a worktree: its parent site name and the
// worktree's unit-slug base joined by "/". Mirrors the watcher's key scheme so
// the UI can look up a worktree's last-active in the shared activity map.
func wtKey(site, wtBase string) string { return site + "/" + wtBase }

// loadIdleActivity reads the watcher's persisted per-key last-active map (unix
// seconds), keyed by site name or worktree key. nil before the watcher writes
// it. Loaded once per sites snapshot so the file is read once, not per site.
func loadIdleActivity() map[string]int64 {
	return idle.LoadActivity(config.IdleActivityFile())
}

// idleSiteIsIdle reports whether a site/worktree key has passed the idle timeout:
// the feature is on, the key isn't paused or pinned, and its last activity (from
// the watcher's persisted map) is older than the timeout.
func idleSiteIsIdle(activity map[string]int64, key string, paused, pinned, enabled bool, timeout time.Duration, now time.Time) bool {
	if !enabled || paused || pinned {
		return false
	}
	ts := activity[key]
	if ts <= 0 {
		return false
	}
	return now.Sub(time.Unix(ts, 0)) >= timeout
}
