//go:build windows

package cli

import "github.com/geodro/lerd/internal/config"

// workerSupportedOnPlatform reports whether the worker can run on this host.
// Container workers run in the machine at the site's /mnt/<drive> path and host
// workers in the site's folder on Windows. Scheduled workers need a timer the
// service manager does not have yet, so they are refused, and returning false
// makes WorkerStartForSite skip the lifecycle calls instead of starting a unit
// that was never written.
var workerSupportedOnPlatform = func(w config.FrameworkWorker) (bool, string) {
	if w.Schedule != "" {
		return false, "scheduled workers aren't supported on Windows yet"
	}
	return true, ""
}

// enableArmsBootOnly is false: Enable on Windows starts the unit now, as on macOS.
var enableArmsBootOnly = false
