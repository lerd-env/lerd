package config

import "runtime"

// PHP runtime modes. Container is the default and the only mode on Linux,
// where a project already shares a filesystem with PHP. Native runs PHP-FPM,
// the CLI and the workers on the macOS host instead, which skips the virtiofs
// crossing a bind-mounted project pays on every file it reads.
const (
	PHPRuntimeContainer = "container"
	PHPRuntimeNative    = "native"
)

// PHPRuntimeMode returns the effective PHP runtime for this install. Anything
// unrecognised, empty, or an absent config normalises to container, so a
// mistyped value can never leave sites pointed at a runtime nothing serves.
func (c *GlobalConfig) PHPRuntimeMode() string {
	return c.phpRuntimeModeOn(runtime.GOOS, runtime.GOARCH)
}

// phpRuntimeModeOn is PHPRuntimeMode with the platform passed in. The native
// runtime exists to skip a virtiofs crossing only macOS pays, and there are no
// native binaries for anything else, so every other platform reports container
// whatever the file says. Refusing to set it is not enough on its own: a
// config.yaml can arrive on a Linux box through a synced home, and honouring it
// there would stop the FPM containers and point every vhost at a listener that
// does not exist.
func (c *GlobalConfig) phpRuntimeModeOn(goos, goarch string) string {
	// Builds exist for Apple silicon only, so an Intel Mac is as incapable of
	// the native runtime as Linux is, and for the same practical reason: there
	// is no binary. Deciding it here rather than at each caller is what makes
	// a config carried onto an unsupported machine harmless.
	if c == nil || goos != "darwin" || goarch != "arm64" {
		return PHPRuntimeContainer
	}
	if c.PHP.Runtime == PHPRuntimeNative {
		return PHPRuntimeNative
	}
	return PHPRuntimeContainer
}

// ServedNatively reports whether a site is served by PHP-FPM on the host under
// the given mode. The mode is global, but it only reaches sites the shared FPM
// container actually serves: FrankenPHP and custom-FPM sites run their own
// containers, a custom container is the user's own definition, and a host-proxy
// site already runs on the host with no PHP runtime of lerd's involved.
func (s *Site) ServedNatively(mode string) bool {
	if mode != PHPRuntimeNative {
		return false
	}
	return !s.IsFrankenPHP() && !s.IsCustomFPM() && !s.IsCustomContainer() && !s.IsHostProxy()
}
