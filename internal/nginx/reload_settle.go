package nginx

import (
	"os"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/podman"
)

// settleTimeout bounds the wait for the previous generation of workers to
// retire. A worker holding a long connection can outlast any budget, so this is
// a ceiling rather than a promise.
const settleTimeout = 5 * time.Second

// settleInterval is how often the worker list is read while settling.
const settleInterval = 100 * time.Millisecond

// nginxWorkerPIDs lists the pids of the workers currently serving. Swappable
// for tests.
var nginxWorkerPIDs = func() ([]string, error) {
	out, err := podman.Run("exec", "lerd-nginx", "sh", "-c",
		"ps -o pid,args | grep 'nginx: worker' | grep -v grep | awk '{print $1}'")
	if err != nil {
		return nil, err
	}
	var pids []string
	for _, line := range strings.Fields(out) {
		pids = append(pids, line)
	}
	return pids, nil
}

// ReloadAndSettle reloads nginx and waits for the workers that were serving the
// previous configuration to retire, so the caller can say the change is live
// without a request landing on a worker that has not heard about it.
//
// `nginx -s reload` only signals the master. The master forks workers on the new
// configuration and lets the old ones drain, so for a moment both generations
// are alive. A domain added in that window is not served yet, and one removed is
// still served, which is what `lerd domain add` and `lerd domain remove` were
// reporting as done.
func ReloadAndSettle() error {
	return reloadAndSettle(Reload, nginxWorkerPIDs, settleTimeout, time.Sleep)
}

// ReloadAndSettleOrWarn is ReloadAndSettle with ReloadOrWarn's error handling:
// a failed reload prints one warning line rather than failing the command.
func ReloadAndSettleOrWarn(indent string) {
	reloadOrWarn(ReloadAndSettle, os.Stdout, indent)
}

// reloadAndSettle is ReloadAndSettle with its dependencies injected. Running out
// of the budget is deliberately not an error: the config on disk is already
// correct and every later request meets it, so failing the command over a worker
// that will not let go would be worse than the window it is closing.
func reloadAndSettle(reload func() error, workers func() ([]string, error), budget time.Duration, tick func(time.Duration)) error {
	before, err := workers()
	if err != nil {
		before = nil
	}
	if err := reload(); err != nil {
		return err
	}
	if len(before) == 0 {
		return nil
	}
	old := make(map[string]bool, len(before))
	for _, pid := range before {
		old[pid] = true
	}
	for waited := time.Duration(0); waited < budget; waited += settleInterval {
		now, err := workers()
		if err != nil {
			return nil
		}
		if !anyStillServing(now, old) {
			return nil
		}
		tick(settleInterval)
	}
	return nil
}

// anyStillServing reports whether any worker from the previous generation is
// still in the list.
func anyStillServing(now []string, old map[string]bool) bool {
	for _, pid := range now {
		if old[pid] {
			return true
		}
	}
	return false
}
