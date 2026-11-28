package config

import "runtime"

// nginx runtime modes. Container is the default: the lerd-nginx container.
// Host runs the host's own nginx, so a native site's requests no longer cross
// the Podman machine twice, in to nginx and back out to PHP-FPM.
const (
	NginxRuntimeContainer = "container"
	NginxRuntimeHost      = "host"
)

// NginxRuntimeMode returns where nginx runs for this install: on the host with
// native PHP, unless kept in its container, and in its container otherwise,
// since a vhost of a containerised PHP reaches FPM by container name.
func (c *GlobalConfig) NginxRuntimeMode() string {
	return c.nginxRuntimeModeOn(runtime.GOOS, runtime.GOARCH)
}

func (c *GlobalConfig) nginxRuntimeModeOn(goos, goarch string) string {
	if c == nil || c.Nginx.Runtime == NginxRuntimeContainer || c.phpRuntimeModeOn(goos, goarch) != PHPRuntimeNative {
		return NginxRuntimeContainer
	}
	return NginxRuntimeHost
}
