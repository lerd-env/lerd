package siteops

import (
	"fmt"
	"time"

	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/unitlog"
)

// frankenPHPPreflight decides whether a runtime switch can go ahead. A failed
// build is survivable only when a usable image is already present: without one
// the quadlet names an image that does not exist, podman reads "localhost" as a
// registry and fails on a TLS handshake, and the unit restart-loops while the
// switch reports success.
func frankenPHPPreflight(phpVersion string, buildErr error, imageExists bool) error {
	if imageExists {
		return nil
	}
	if buildErr != nil {
		return fmt.Errorf("building the FrankenPHP image for PHP %s: %w", phpVersion, buildErr)
	}
	return fmt.Errorf("the FrankenPHP image for PHP %s is missing after the build; run 'lerd php:rebuild' and try again", phpVersion)
}

// frankenPHPUnitError turns the unit's actual state into the switch's result.
// systemctl start returns before a quadlet unit has settled, so a switch that
// trusts it reports success over a container that never came up.
func frankenPHPUnitError(unit string, running bool) error {
	if running {
		return nil
	}
	return fmt.Errorf("the FrankenPHP container did not start; inspect it with '%s' and switch back with 'lerd runtime fpm'", unitlog.LogHint(unit))
}

// waitContainerRunningFn reports whether the container settles into a running
// state. A quadlet unit that is going to fail does so within a couple of restart
// attempts, so a short bounded wait tells success from failure without making a
// healthy switch feel slow.
var waitContainerRunningFn = func(name string) bool {
	deadline := time.Now().Add(frankenPHPStartTimeout)
	for time.Now().Before(deadline) {
		if running, _ := podman.ContainerRunning(name); running {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

// frankenPHPStartTimeout bounds the wait for the container to come up. Long
// enough for a cold start, short enough that a restart-looping unit is reported
// rather than waited on.
const frankenPHPStartTimeout = 15 * time.Second
