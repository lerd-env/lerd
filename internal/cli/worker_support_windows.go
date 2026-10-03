//go:build windows

package cli

import "github.com/geodro/lerd/internal/config"

// workerSupportedOnPlatform reports whether the worker can run on this host.
// None can yet, see errWorkersNeedPathMapping, and returning false makes
// WorkerStartForSite skip the lifecycle calls instead of starting a unit that
// was never written.
var workerSupportedOnPlatform = func(_ config.FrameworkWorker) (bool, string) {
	return false, errWorkersNeedPathMapping.Error()
}

// enableArmsBootOnly is false: Enable on Windows starts the unit now, as on macOS.
var enableArmsBootOnly = false
