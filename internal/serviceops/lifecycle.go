package serviceops

import (
	"fmt"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
)

// StartService is the shared start path for CLI, UI, TUI (via CLI), and MCP:
// ensure the quadlet, bring depends_on satisfiers up for customs, start the
// unit (retrying briefly for quadlet generator lag), mark it manually started,
// start reverse dependents, and regenerate discover_family consumers.
func StartService(name string) error {
	unit := "lerd-" + name
	if IsBuiltin(name) {
		if err := EnsureDefaultPresetQuadlet(name); err != nil {
			return err
		}
	} else {
		svc, err := config.LoadCustomService(name)
		if err != nil {
			if config.PresetExists(name) {
				return fmt.Errorf("service %q is not installed; install it with 'lerd service preset %s'", name, name)
			}
			return fmt.Errorf("unknown service %q", name)
		}
		if err := StartDependencies(svc); err != nil {
			return err
		}
		if err := EnsureCustomServiceQuadlet(svc); err != nil {
			return err
		}
	}
	if err := startUnitRetry(unit); err != nil {
		return err
	}
	_ = config.SetServicePaused(name, false)
	_ = config.SetServiceManuallyStarted(name, true)

	// Bring up admin UIs (and similar) that declare a depends_on this service
	// can satisfy — family / env_role aware, not literal name only.
	for _, dep := range dependentsOf(name) {
		if err := EnsureServiceRunning(dep); err != nil {
			feedback.Warn("could not start dependent service %s: %v", dep, err)
		}
	}
	RegenerateFamilyConsumersForService(name)
	return nil
}

// StopService is the shared stop path for CLI, UI, TUI (via CLI), and MCP:
// cascade-stop dependents when no other running satisfier remains, stop name,
// mark it paused, and regenerate discover_family consumers.
func StopService(name string) error {
	StopWithDependents(name)
	_ = config.SetServicePaused(name, true)
	_ = config.SetServiceManuallyStarted(name, false)
	RegenerateFamilyConsumersForService(name)
	return nil
}

// RestartService is the shared restart path for CLI, UI, TUI (via CLI), and MCP:
// refresh the quadlet (so discover_family and file mounts land), restart the
// unit, clear paused, and regenerate discover_family consumers.
func RestartService(name string) error {
	unit := "lerd-" + name
	if err := refreshServiceQuadlet(name); err != nil {
		feedback.Warn("regenerating quadlet for %s failed: %v; restarting with the existing one", name, err)
	}
	if err := podman.RestartUnit(unit); err != nil {
		return err
	}
	_ = config.SetServicePaused(name, false)
	_ = config.SetServiceManuallyStarted(name, true)
	RegenerateFamilyConsumersForService(name)
	return nil
}

func refreshServiceQuadlet(name string) error {
	if IsBuiltin(name) {
		return EnsureDefaultPresetQuadlet(name)
	}
	svc, err := config.LoadCustomService(name)
	if err != nil {
		return err
	}
	return EnsureCustomServiceQuadlet(svc)
}

// startUnitRetry starts a unit, retrying briefly when the generator has not
// published the unit yet after a daemon-reload (UI and install share this).
func startUnitRetry(unit string) error {
	var err error
	for attempt := range 5 {
		err = podman.StartUnit(unit)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
	}
	return err
}
