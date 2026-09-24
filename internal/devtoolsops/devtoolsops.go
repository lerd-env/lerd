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
)

// WorkersResult reports the post-apply state of the worker-capture toggle.
type WorkersResult struct {
	Workers  bool
	NoChange bool
}

// SetWorkers persists whether the dashboard and TUI show worker events. It
// is a view setting: worker capture is always on (see EnsureDevtoolsAssets),
// so switching it never loses what a worker did while hidden.
func SetWorkers(enabled bool) (WorkersResult, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return WorkersResult{}, fmt.Errorf("loading config: %w", err)
	}
	if cfg.IsDevtoolsWorkers() == enabled {
		return WorkersResult{Workers: enabled, NoChange: true}, nil
	}
	cfg.SetDevtoolsWorkers(enabled)
	if err := config.SaveGlobal(cfg); err != nil {
		return WorkersResult{Workers: !enabled}, fmt.Errorf("saving config: %w", err)
	}
	return WorkersResult{Workers: enabled}, nil
}
