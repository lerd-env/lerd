package nginx

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// hostGateway is how a container reaches a process on the macOS host.
const hostGateway = podman.HostGateway

// fpmUpstream returns the host and port nginx fastcgi's to for a site.
func fpmUpstream(site *config.Site, phpVersion string) (string, int) {
	return podman.FPMUpstream(*site, phpVersion)
}
