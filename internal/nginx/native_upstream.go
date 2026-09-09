package nginx

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/podman"
)

// containerFPMPort is the port the shared and per-site FPM containers listen on.
const containerFPMPort = 9000

// hostGateway is how a container reaches a process on the macOS host. nginx
// stays containerised in native mode, so it cannot use loopback: 127.0.0.1
// inside the container is the container.
const hostGateway = "host.containers.internal"

// fpmUpstream returns the host and port nginx fastcgi's to for a site. A native
// site is served by a PHP-FPM on the host, reached through the container's host
// gateway on the port its PHP version owns; every other site keeps its FPM
// container on 9000. An unparseable version falls back to the container so a
// bad value can never render a vhost pointing at port 0.
func fpmUpstream(site *config.Site, phpVersion string) (string, int) {
	container := podman.FPMContainerName(*site, phpVersion)
	if !site.IsNative() {
		return container, containerFPMPort
	}
	port, err := nativephp.PortFor(phpVersion)
	if err != nil {
		return container, containerFPMPort
	}
	return hostGateway, port
}
