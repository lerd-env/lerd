// Package dumpsops contains the shared business logic for toggling the lerd
// dump bridge. Implementation is restart-free: the bridge PHP file and its
// conf.d ini are always volume-mounted into every FPM container, and the
// active/inactive state is signalled by a sentinel file inside the same
// mount. Toggling is a single filesystem touch and applies on the next
// PHP request without any container or worker disruption.
package dumpsops

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// Result describes the outcome of Apply so callers can render their own
// user-facing message without inspecting state again.
type Result struct {
	Enabled  bool // post-apply state
	NoChange bool // requested state already matched; no FS changes
}

// Apply flips Dumps.Enabled to the requested state. Persists the config
// flag, ensures the bridge assets exist on disk, and touches/removes the
// runtime sentinel that controls bridge behaviour. No FPM containers are
// restarted because the assets are always volume-mounted; the bridge
// reads the sentinel on each request.
//
// Idempotent: a second call with the same value returns NoChange=true
// without touching the filesystem.
func Apply(enabled bool) (Result, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return Result{}, fmt.Errorf("loading config: %w", err)
	}

	if cfg.IsDumpsEnabled() == enabled {
		return Result{Enabled: enabled, NoChange: true}, nil
	}

	cfg.SetDumpsEnabled(enabled)
	if err := config.SaveGlobal(cfg); err != nil {
		return Result{Enabled: enabled}, fmt.Errorf("saving config: %w", err)
	}

	// Always make sure the bridge files exist on disk. Even when the user
	// is turning the bridge OFF, lerd-ui must keep them there because the
	// FPM quadlet has them as bind-mount sources — removing them would
	// make podman auto-create directories at those paths on the next FPM
	// start.
	if err := podman.WriteDumpBridgeAssets(); err != nil {
		return Result{Enabled: enabled}, fmt.Errorf("writing dump assets: %w", err)
	}

	if err := podman.SetDumpsBridgeFlag(enabled); err != nil {
		return Result{Enabled: enabled}, err
	}

	return Result{Enabled: enabled}, nil
}
