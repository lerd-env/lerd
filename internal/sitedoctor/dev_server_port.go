package sitedoctor

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/freeport"
	"github.com/geodro/lerd/internal/podman"
)

var (
	devServerPortFree = freeport.Bindable
	devServerStatus   = podman.UnitStatus
)

// checkDevServerPort fails when another process holds the port of a host-proxy
// site whose dev server is down: the site then answers 200 with that app.
func checkDevServerPort(path string, fw *config.Framework) (Check, bool) {
	site, err := config.FindSiteByPath(path)
	if err != nil || site == nil || !site.IsHostProxy() || site.HostCommand == "" {
		return Check{}, false
	}
	c := Check{Name: "dev_server_port", Status: StatusOK}
	if st, _ := devServerStatus(config.HostProxyWorkerUnit(site.Name)); st == "active" || devServerPortFree(site.HostPort) {
		return c, true
	}
	c.Status = StatusFail
	c.Detail = fmt.Sprintf("another process has port %d, so %s cannot start on it. Free it or change proxy.port in .lerd.yaml and run `lerd link`.", site.HostPort, site.Name)
	if p := (&config.ProxyConfig{Command: site.HostCommand, Port: site.HostPort}); p.CanMovePort(fw) {
		c.Detail = fmt.Sprintf("another process has port %d. Run `lerd restart` to start %s on a free one.", site.HostPort, site.Name)
	}
	return c, true
}
