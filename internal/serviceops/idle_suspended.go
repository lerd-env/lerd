package serviceops

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// SetIdleSuspended marks or clears the idle-suspend flag for each service and
// keeps its boot start in step: a sleeping service's quadlet loses [Install],
// so a reboot leaves it down for the next request to wake. Only a boot reads
// that, so the reload is deferred to the next one rather than paid here.
func SetIdleSuspended(names []string, asleep bool) {
	changed := false
	for _, name := range names {
		_ = config.SetServiceIdleSuspended(name, asleep)
		if podman.SetQuadletAutostart("lerd-"+name, !asleep) {
			changed = true
		}
	}
	if changed {
		podman.DeferDaemonReload()
	}
}
