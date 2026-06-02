// Package devtoolsops contains the shared logic for toggling the lerd_devtools
// collector. Like the debug bridge, it is restart-free: the conf.d ini is
// always volume-mounted into every FPM container and active state is signalled
// by a sentinel file the extension stats per request, so toggling is a single
// filesystem touch that applies on the next PHP request without restarting any
// container or worker.
package devtoolsops

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// WorkersResult reports the post-apply state of the worker-capture toggle.
type WorkersResult struct {
	Workers  bool
	NoChange bool
}

// SetWorkers opts queue/scheduler worker queries into (or out of) capture by
// touching the workers sentinel and persisting the flag. Restart-free, same as
// the enable toggle. Long-running workers pick up the change when they next
// recycle (Horizon) or run (scheduler); a one-off restart isn't forced.
func SetWorkers(enabled bool) (WorkersResult, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return WorkersResult{}, fmt.Errorf("loading config: %w", err)
	}
	if cfg.IsDevtoolsWorkers() == enabled {
		return WorkersResult{Workers: enabled, NoChange: true}, nil
	}
	if enabled {
		if err := podman.SetDevtoolsWorkersFlag(true); err != nil {
			return WorkersResult{Workers: false}, err
		}
		cfg.SetDevtoolsWorkers(true)
		if err := config.SaveGlobal(cfg); err != nil {
			_ = podman.SetDevtoolsWorkersFlag(false)
			return WorkersResult{Workers: false}, fmt.Errorf("saving config: %w", err)
		}
		return WorkersResult{Workers: true}, nil
	}
	cfg.SetDevtoolsWorkers(false)
	if err := config.SaveGlobal(cfg); err != nil {
		return WorkersResult{Workers: true}, fmt.Errorf("saving config: %w", err)
	}
	if err := podman.SetDevtoolsWorkersFlag(false); err != nil {
		return WorkersResult{Workers: false}, err
	}
	return WorkersResult{Workers: false}, nil
}
