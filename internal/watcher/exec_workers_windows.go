//go:build windows

package watcher

import "time"

// WatchExecWorkers is a no-op on Windows: workers have no guard scripts to heal.
func WatchExecWorkers(_ time.Duration) {}
