package nginx

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// nativeUpstreamsFile defines one upstream per PHP version for native sites.
// A FastCGI connection opened per request crosses gvproxy, which now and then
// drops one whose port comes round again: a 504 after fastcgi_connect_timeout.
const nativeUpstreamsFile = "_native-upstreams.conf"

// nativeKeepalive is how many idle connections each nginx worker keeps per
// pool. Fewer churned enough of them to run the Mac out of ports under load;
// nativephp's pm.max_children leaves room for this many on 16 workers.
const nativeKeepalive = 8

func nativeUpstreamName(version string) string { return "lerd_native_php" + phpShort(version) }

// renderNativeUpstreams lists every version a site can run, so the file never
// changes when one switches PHP version.
func renderNativeUpstreams() string {
	var b strings.Builder
	b.WriteString("# Managed by lerd: the native PHP pools, reused across requests.\n")
	for _, v := range config.SupportedPHPVersions {
		port, err := nativephp.PortFor(v)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "upstream %s {\n    server %s:%d;\n    keepalive %d;\n    keepalive_timeout 10s;\n}\n",
			nativeUpstreamName(v), hostGateway, port, nativeKeepalive)
	}
	return b.String()
}

// NativeUpstream is the upstream a native vhost passes to, or "" for one served
// by a container or on a version the upstreams file does not list.
func (d VhostData) NativeUpstream() string {
	if d.FPMContainer != hostGateway {
		return ""
	}
	for _, v := range config.SupportedPHPVersions {
		if port, err := nativephp.PortFor(v); err == nil && port == d.FPMPort {
			return nativeUpstreamName(v)
		}
	}
	return ""
}

// ensureNativeUpstreamsFor writes the upstreams file before a vhost that names
// one of them: nginx refuses the whole config over one undefined upstream.
func ensureNativeUpstreamsFor(rendered []byte) error {
	if !bytes.Contains(rendered, []byte("fastcgi_pass lerd_native_php")) {
		return nil
	}
	path := filepath.Join(config.NginxConfD(), nativeUpstreamsFile)
	want := []byte(renderNativeUpstreams())
	if have, err := os.ReadFile(path); err == nil && bytes.Equal(have, want) {
		return nil
	}
	config.GuardRealWrite(path)
	return WriteFileAtomic(path, want, 0644)
}
