//go:build windows

package cli

import (
	"errors"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/unitlog"
)

// errWorkersNeedPathMapping is why workers are off on Windows for now: a worker
// bind-mounts the site at the same path inside its container, and a Windows path
// such as C:\Sites\app has no such twin inside the Linux VM until the host to
// VM path mapping exists.
var errWorkersNeedPathMapping = errors.New("workers are not available on Windows yet: they need the host to VM path mapping")

func writeWorkerUnitFile(_, _, _, _, _, _, _, _, _, _ string, _ bool) (bool, error) {
	return false, errWorkersNeedPathMapping
}

func workerLogHint(unitName string, host bool) string {
	if !host {
		if cfg, _ := config.LoadGlobal(); cfg != nil && cfg.WorkerExecMode() == config.WorkerExecModeContainer {
			return "podman logs -f " + unitName
		}
	}
	return unitlog.LogHint(unitName)
}

func removeWorkerExecArtifacts(_ string) {}

func restoreWorker(_, _, _, _ string, _ config.FrameworkWorker) {}

func migrateWorkersOnModeChangeStreaming(_, _ string, _ func(WorkerModePhaseEvent)) error {
	return nil
}
