//go:build linux

package watcher

import "time"

// WatchExecWorkers is a no-op on Linux. Workers run as systemd services
// with Restart=always under their own supervisor — no out-of-band heal
// needed and no podman-machine bridge to wedge.
func WatchExecWorkers(_ time.Duration) {}
