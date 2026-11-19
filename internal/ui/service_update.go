package ui

import "github.com/geodro/lerd/internal/serviceops"

// Seams so the cached update answer can be tested without podman or a registry.
var (
	checkServiceUpdateFn   = serviceops.CheckUpdateAvailable
	refreshServiceUpdateFn = serviceops.RefreshUpdateAvailability
)

// serviceUpdate is a service's update answer from lerd-ui's hourly cache, nil
// when it cannot be read. Its unit's restart, an update included, drops it.
func serviceUpdate(name string) *serviceops.UpdateAvailability {
	return cachedUpdate(updates, "lerd-"+name, func() *serviceops.UpdateAvailability {
		avail, err := checkServiceUpdateFn(name)
		if err != nil {
			return nil
		}
		return avail
	})
}
