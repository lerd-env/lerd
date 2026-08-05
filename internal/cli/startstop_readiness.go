package cli

import (
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/podman"
)

// serviceReadyTimeout bounds how long a start waits for an engine to accept
// connections. Long enough for a cold mysql on a slow disk, short enough that a
// wedged engine cannot hold the whole start hostage.
const serviceReadyTimeout = 30 * time.Second

// nonEngineUnits are started alongside the engines but answer no readiness
// probe, so waiting on them would add latency to every start for nothing.
var nonEngineUnits = map[string]bool{"lerd-ui": true, "lerd-watcher": true, "lerd-nginx": true, "lerd-dns": true}

// serviceNamesForReadiness turns the started unit list into the service names
// worth probing: the engines, without the daemons and timers that have nothing
// to answer with.
func serviceNamesForReadiness(units []string) []string {
	var names []string
	for _, u := range units {
		if nonEngineUnits[u] || strings.HasSuffix(u, ".timer") {
			continue
		}
		if name := strings.TrimPrefix(u, "lerd-"); name != u && name != "" {
			names = append(names, name)
		}
	}
	return names
}

// waitServicesReady blocks until every started engine accepts connections or
// the timeout elapses. A service that never comes up is reported and skipped
// rather than failing the start: the rest of the stack is still usable, and
// `lerd status` and `lerd doctor` describe the failure properly.
func waitServicesReady(units []string, timeout time.Duration) {
	names := serviceNamesForReadiness(units)
	if len(names) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, n := range names {
		wg.Add(1)
		go func(service string) {
			defer wg.Done()
			_ = podman.WaitReady(service, timeout)
		}(n)
	}
	wg.Wait()
}
