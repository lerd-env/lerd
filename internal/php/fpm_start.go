package php

import (
	"errors"
	"fmt"
	"time"

	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/services"
)

// ErrFPMNotInstalled is returned (wrapped) by StartFPM when the requested
// version's FPM container does not exist yet, so the caller knows the fix is to
// install it rather than just start it. Test with errors.Is.
var ErrFPMNotInstalled = errors.New("FPM container is not installed")

// FPMInstalled reports whether the FPM container already exists (so it only
// needs starting) rather than needing a fresh build. It checks both the shared
// per-version units and the specific container's unit, so a custom-FPM container
// (lerd-cfpm-<site>, whose version isn't in the shared list) is recognised too.
func FPMInstalled(version, container string) bool {
	return IsInstalled(version) || services.Mgr.ContainerUnitInstalled(container)
}

// StartFPM brings up a stopped-but-installed FPM container and waits for it to
// report running, so an exec right after doesn't race the boot. It is a no-op
// when the container is already running and returns ErrFPMNotInstalled (wrapped
// with the version) when the container doesn't exist yet. It produces no output;
// callers add any user-facing progress. Shared by the CLI php/artisan/shell
// commands and the MCP exec handlers so both auto-start the same way.
func StartFPM(version, container string) error {
	if running, _ := podman.ContainerRunning(container); running {
		return nil
	}
	if !FPMInstalled(version, container) {
		return fmt.Errorf("PHP %s: %w", version, ErrFPMNotInstalled)
	}
	if err := podman.StartUnit(container); err != nil {
		return fmt.Errorf("starting PHP %s FPM: %w", version, err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		if running, _ := podman.ContainerRunning(container); running {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s to start", container)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
