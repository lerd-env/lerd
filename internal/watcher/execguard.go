package watcher

import (
	"time"

	"github.com/geodro/lerd/internal/execguard"
)

// WatchExecGuard kills what lerd runs left running in a container after the
// lerd that started them was killed outright, so a test run cut short by a
// timeout or a closed session does not keep its browser up and its site awake.
func WatchExecGuard(interval time.Duration) {
	execguard.Sweep()
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		execguard.Sweep()
	}
}
