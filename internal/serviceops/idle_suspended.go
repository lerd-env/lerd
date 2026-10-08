package serviceops

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// SetIdleSuspended flags each service asleep or awake and keeps its boot start
// in step, so a reboot leaves a sleeping one down. Only a boot reads that, so
// the reload is deferred to the next one rather than paid here.
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
