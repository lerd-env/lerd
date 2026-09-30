package serviceops

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/imagepull"
	"github.com/geodro/lerd/internal/podman"
)

// StartService is the shared start path for CLI, UI, TUI (via CLI), and MCP:
// ensure the quadlet, bring depends_on satisfiers up for customs, start the
// unit (retrying briefly for quadlet generator lag), mark it manually started,
// start reverse dependents, and regenerate dynamic_env consumers.
func StartService(name string) error {
	unit := "lerd-" + name
	if IsBuiltin(name) {
		wasInstalled := ServiceInstalled(name)
		if err := EnsureDefaultPresetQuadlet(name); err != nil {
			return err
		}
		if err := pullStartImage(name); err != nil {
			// A first start that cannot fetch its image leaves nothing behind.
			if !wasInstalled {
				_ = podman.RemoveQuadlet(unit)
				_ = podman.DaemonReloadFn()
			}
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
		if err := pullStartImage(name); err != nil {
			return err
		}
	}
	if err := startUnitRetry(unit); err != nil {
		return err
	}
	_ = config.SetServicePaused(name, false)
	_ = config.SetServiceManuallyStarted(name, true)
	_ = config.SetServiceRemoved(name, false)

	// Bring up admin UIs (and similar) that declare a depends_on this service
	// can satisfy — family / env_role aware, not literal name only.
	for _, dep := range dependentsOf(name) {
		if err := EnsureServiceRunning(dep); err != nil {
			feedback.Warn("could not start dependent service %s: %v", dep, err)
		}
	}
	RegenerateDynamicEnvConsumersForService(name)
	// Starting is where the port guard shifts a service off a port something
	// else took while it was down, and where a service installed since the last
	// start first needs its dashboard served.
	syncDashboardVhost()
	return nil
}

// StopService is the shared stop path for CLI, UI, TUI (via CLI), and MCP:
// cascade-stop dependents when no other running satisfier remains, stop name,
// mark it paused, and regenerate dynamic_env consumers.
func StopService(name string) error {
	if err := StopWithDependents(name); err != nil {
		return err
	}
	_ = config.SetServicePaused(name, true)
	_ = config.SetServiceManuallyStarted(name, false)
	RegenerateDynamicEnvConsumersForService(name)
	return nil
}

// RestartService is the shared restart path for CLI, UI, TUI (via CLI), and MCP:
// refresh the quadlet (so dynamic_env and file mounts land), restart the
// unit, clear paused, and regenerate dynamic_env consumers.
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
	RegenerateDynamicEnvConsumersForService(name)
	syncDashboardVhost()
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

// Seams so tests can drive a missing image without podman or a registry.
var (
	imageExistsFn = podman.ImageExists
	imageSizeFn   = imagepull.Size
	pullImageFn   = func(img string) error { return podman.PullImageTo(img, os.Stdout) }
)

// StartImage is the image starting name runs: the one its unit or config
// records, or for a built-in not installed yet, the one installing it writes.
func StartImage(name string) string {
	if current, _ := serviceImageRefs(name); current != "" {
		return current
	}
	if IsBuiltin(name) {
		if svc, err := resolveDefaultPresetService(name, ""); err == nil {
			return svc.Image
		}
	}
	return ""
}

// pullStartImage fetches a missing image before the unit starts. Left to the
// start, podman pulls it silently, with no size and no way to say no.
func pullStartImage(name string) error {
	img := StartImage(name)
	if img == "" || imageExistsFn(img) {
		return nil
	}
	bytes, _ := imageSizeFn(img)
	fmt.Printf("  Pulling %s%s for %s\n", img, imagepull.Note(bytes), name)
	return pullImageFn(img)
}
