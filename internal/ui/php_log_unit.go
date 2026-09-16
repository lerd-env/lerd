package ui

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
)

// phpLogUnit names the unit the site's PHP log tab streams, and only under the
// native runtime: the shared FPM container is stopped by design there, so the
// tab has to read the host listener's launchd log instead of a container that
// is not there. Empty otherwise, which leaves the dashboard naming the
// container itself. Naming one here instead answered for every site with the
// shared FPM, which is not what a FrankenPHP or custom-container site reads.
func phpLogUnit(site config.Site, phpVersion string, native bool) string {
	if native && site.ServedNatively(config.PHPRuntimeNative) {
		return nativephp.UnitLabel(phpVersion)
	}
	return ""
}

// nativeRuntimeActive reports whether this install serves PHP from the host.
// A var so a test can pin the runtime the version actions branch on without
// writing a global config.
var nativeRuntimeActive = func() bool {
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

// nativeListenerRunning reports whether the host FPM for a version is up. The
// dashboard polls this continuously, so it reads the pool's launchd job rather
// than dialling its port: a dial is handed to a child and resets the ondemand
// idle timer, so no pool would ever fall to zero while a dashboard is open. It
// is the same question the container side answers with "is the container
// running"; `lerd status` still dials, because a person asked it to.
func nativeListenerRunning(version string) bool { return nativephp.Loaded(version) }

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

// nativeRuntimeApplies reports whether this machine could use the native
// runtime at all, which is what decides whether the dashboard offers the
// toggle. Builds ship for both macOS architectures, so the platform is the
// whole test; goarch is still taken so narrowing it stays a one-line change.
func nativeRuntimeApplies(goos, goarch string) bool {
	_ = goarch
	return goos == "darwin"
}

// reportedRuntime is the runtime the dashboard sees for a site. The frontend
// keys "does this site have a container" off it, and already treats "native" as
// none; nothing ever set that, because native is an install-wide mode rather
// than a per-site one. Only a plain FPM site moves: FrankenPHP, custom FPM,
// custom containers and host proxies keep their own runtime, since the switch
// never took them off theirs.
func reportedRuntime(siteRuntime string, native bool, containerPort, hostPort int) string {
	if native && siteRuntime == "" && containerPort == 0 && hostPort == 0 {
		return "native"
	}
	return siteRuntime
}
