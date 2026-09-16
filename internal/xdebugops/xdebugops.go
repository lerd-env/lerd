// Package xdebugops contains the shared business logic for toggling Xdebug
// on a PHP version: mode validation, config persistence, ini write, FPM
// quadlet update, and unit restart. The CLI, UI, and MCP all call into here
// so the three surfaces stay in lockstep on state transitions and ordering.
package xdebugops

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/podman"
)

// Result describes the outcome of Apply so callers can render their own
// user-facing message without inspecting the config again.
type Result struct {
	Version   string
	Mode      string // canonical mode after Apply; "" means xdebug disabled
	Enabled   bool   // convenience: Mode != ""
	NoChange  bool   // true when the requested state already matched; no restart was attempted
	Restarted bool   // true when the FPM unit restart succeeded
	// RestartErr is set when the FPM unit restart failed. Non-fatal: config and
	// ini are already persisted, the caller just needs to surface a hint so
	// the user can restart the unit manually.
	RestartErr error
}

// Apply toggles Xdebug for version with the default start_with_request=yes
// (connect on every request). See ApplyWithStart for on-demand modes.
func Apply(version, rawMode string) (Result, error) {
	return ApplyWithStart(version, rawMode, "yes")
}

// ApplyWithStart toggles Xdebug for version and sets its start_with_request
// value (yes | trigger | no). An empty mode disables xdebug; a non-empty mode
// is validated via podman.NormaliseXdebugMode. With "trigger"/"no", debugging
// is driven on demand (a trigger cookie, or the control socket via
// `lerd xdebug pause`) instead of every request and worker connecting. It is
// idempotent: passing the current mode and start returns NoChange=true.
func ApplyWithStart(version, rawMode, start string) (Result, error) {
	targetMode := ""
	if rawMode != "" {
		m, err := podman.NormaliseXdebugMode(rawMode)
		if err != nil {
			return Result{Version: version}, err
		}
		targetMode = m
	}
	if start == "" {
		start = "yes"
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return Result{Version: version}, fmt.Errorf("loading config: %w", err)
	}

	// Disabled state ignores start; enabled state must match both to be a no-op.
	if cfg.GetXdebugMode(version) == targetMode && (targetMode == "" || cfg.GetXdebugStart(version) == start) {
		return Result{
			Version:  version,
			Mode:     targetMode,
			Enabled:  targetMode != "",
			NoChange: true,
		}, nil
	}

	cfg.SetXdebugMode(version, targetMode)
	if targetMode == "" {
		cfg.SetXdebugStart(version, "")
	} else {
		cfg.SetXdebugStart(version, start)
	}
	if err := config.SaveGlobal(cfg); err != nil {
		return Result{Version: version}, fmt.Errorf("saving config: %w", err)
	}

	if err := podman.WriteXdebugIni(version, targetMode, start); err != nil {
		return Result{Version: version}, fmt.Errorf("writing xdebug ini: %w", err)
	}

	res, err := applyRestart(version, cfg.PHPRuntimeMode())
	if err != nil {
		return Result{Version: version}, err
	}
	res.Mode = targetMode
	res.Enabled = targetMode != ""
	return res, nil
}

// Seams for the restart step, so the runtime split is testable without launchd
// or podman.
var (
	writeFPMQuadlet  = podman.WriteFPMQuadlet
	restartFPMUnit   = podman.RestartUnit
	reloadNativePool = nativephp.Reload
)

// applyRestart makes a changed xdebug ini reach the processes serving requests.
// Under the native runtime that is the version's host pool; there is no FPM
// container, and writing its quadlet then restarting it fails against an image
// that was never built.
func applyRestart(version, mode string) (Result, error) {
	res := Result{Version: version}
	if mode == config.PHPRuntimeNative {
		if err := reloadNativePool(version); err != nil {
			res.RestartErr = err
			return res, nil
		}
		res.Restarted = true
		return res, nil
	}
	if err := writeFPMQuadlet(version); err != nil {
		return res, fmt.Errorf("writing FPM quadlet: %w", err)
	}
	if err := restartFPMUnit(FPMUnit(version)); err != nil {
		res.RestartErr = err
		return res, nil
	}
	res.Restarted = true
	return res, nil
}

// FPMUnit returns the systemd unit name for a PHP version's FPM container.
// Exposed so callers can print consistent "Run: systemctl --user restart ..."
// hints when Apply's RestartErr is non-nil.
func FPMUnit(version string) string {
	return "lerd-php" + strings.ReplaceAll(version, ".", "") + "-fpm"
}
