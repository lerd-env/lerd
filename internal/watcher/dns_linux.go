package watcher

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dns"
)

// wireOSDNSDeps turns on the repairs specific to rootless podman on the host.
// Container DNS re-sync recovers from aardvark-dns forwarder staleness, and
// nginx is restarted after a host resume leaves rootless networking in a bad
// state, so .test sites stop returning "Secure Connection Failed" until a
// manual lerd restart (issue #665).
func wireOSDNSDeps(deps *dnsWatchDeps, tld string) {
	deps.dnsEnvFingerprint = defaultDNSEnvFingerprint
	deps.resyncContainerDNS = defaultResyncContainerDNS
	deps.nginxHealthy = defaultNginxHealthy
	deps.repairNginx = defaultRepairNginx
	deps.dnsDaemonAnswering = func() bool { return dns.DaemonAnswering(tld) }
	deps.repairDNS = defaultRepairDNS
	deps.isStopped = config.IsStopped
}
