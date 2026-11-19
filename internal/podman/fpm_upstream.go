package podman

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/platform"
)

// containerFPMPort is the port the shared and per-site FPM containers listen on.
const containerFPMPort = 9000

// HostGateway is how a container reaches a process on the macOS host. The web
// server stays containerised in native mode, so it cannot use loopback.
const HostGateway = "host.containers.internal"

// FPMUpstream returns where a web server sends a site's PHP: a native site's
// host PHP-FPM on its version's port, else its FPM container on 9000. A bad
// version falls back to the container, so it never points at port 0.
func FPMUpstream(site config.Site, phpVersion string) (string, int) {
	container := FPMContainerName(site, phpVersion)
	if !site.IsNative() {
		return container, containerFPMPort
	}
	port, err := nativephp.PortFor(phpVersion)
	if err != nil {
		return container, containerFPMPort
	}
	return HostGateway, port
}

// HostProxyUpstream is the address a web server in a container reaches a
// process on the host at: gvproxy's name on macOS, else the routable gateway
// the probe cached in the hosts file (a pure read, no podman call).
func HostProxyUpstream() string {
	if platform.Current.UsesMachineVM {
		return HostGateway
	}
	if ip := ReadHostGatewayFromFile(); ip != "" {
		return ip
	}
	return HostGateway
}
