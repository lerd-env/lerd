package siteops

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// RerunSiteEnv runs `lerd env` in a site's directory. The cli package owns the
// env step and wires it at init, the same way as StopRuntimeWorkers.
var RerunSiteEnv func(dir string)

// SyncEnvAfterRuntimeSwitch reruns lerd env when a runtime switch moved a site's
// PHP between the host and a container under native, since they reach services
// differently. Octane reads .env at boot, so a FrankenPHP worker restarts.
func SyncEnvAfterRuntimeSwitch(site *config.Site, wasNative bool) bool {
	if site.IsNative() == wasNative || RerunSiteEnv == nil {
		return false
	}
	RerunSiteEnv(site.Path)
	if site.IsFrankenPHP() && site.RuntimeWorker {
		unit := podman.FrankenPHPContainerName(site.Name)
		if err := podman.RestartUnit(unit); err != nil {
			fmt.Printf("[WARN] restarting %s for the new .env: %v\n", unit, err)
		}
	}
	return true
}
