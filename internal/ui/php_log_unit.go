package ui

import (
	"fmt"
	"net"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/podman"
)

// phpLogUnit names the unit the site's PHP log tab streams. Under the native
// runtime the shared FPM container is stopped by design, so the tab has to read
// the host listener's launchd log instead of a container that is not there.
// FrankenPHP and custom-FPM sites keep their own container either way.
func phpLogUnit(site config.Site, phpVersion string, native bool) string {
	if native && !site.IsFrankenPHP() && !site.IsCustomFPM() && !site.IsCustomContainer() && !site.IsHostProxy() {
		return nativephp.UnitLabel(phpVersion)
	}
	return podman.FPMContainerName(site, phpVersion)
}

// nativeRuntimeActive reports whether this install serves PHP from the host.
func nativeRuntimeActive() bool {
	cfg, err := config.LoadGlobal()
	return err == nil && cfg.PHPRuntimeMode() == config.PHPRuntimeNative
}

// phpVersionRunning reports whether a PHP version is actually serving, asking
// whichever runtime this install uses. Reading container state under the native
// runtime would paint the dashboard grey while PHP is serving from the host.
func phpVersionRunning(version string, native bool, containerRunning, listenerRunning func(string) bool) bool {
	if native {
		return listenerRunning(version)
	}
	return containerRunning(version)
}

// nativeListenerRunning reports whether the host FPM for a version is accepting
// connections on its port.
func nativeListenerRunning(version string) bool {
	port, err := nativephp.PortFor(version)
	if err != nil {
		return false
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// installedPHPVersions lists the PHP versions this install can actually serve
// with, asking whichever runtime is active. Returns an empty slice rather than
// nil so the dashboard renders a list either way.
func installedPHPVersions(native bool, container, host func() []string) []string {
	var out []string
	if native {
		out = host()
	} else {
		out = container()
	}
	if out == nil {
		return []string{}
	}
	return out
}

// nativeInstallRefusal returns the error a PHP install should fail with under
// the native runtime. Installing here means fetching a prebuilt binary rather
// than building an image, and until those are published there is nothing to
// fetch; building the image anyway would produce an artifact this runtime
// never uses.
func nativeInstallRefusal(native bool, version string) error {
	if !native {
		return nil
	}
	return fmt.Errorf("cannot install PHP %s under the native runtime: it needs a prebuilt native binary, not a container image. Switch with 'lerd php:runtime container' to install one", version)
}
