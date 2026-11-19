package nginx

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// ApplySite writes the vhost serving the site's own domain for its kind, over
// HTTPS when it is secured, on the global default PHP when the site names
// none. Paused and waking pages are written elsewhere.
func ApplySite(site config.Site) error {
	name := site.PrimaryDomain() + ".conf"
	switch {
	case site.IsHostProxy():
		tmpl := "vhost-hostproxy.conf.tmpl"
		if site.Secured {
			tmpl = "vhost-hostproxy-ssl.conf.tmpl"
		}
		rendered, err := renderHostProxyVhost(site, tmpl, site.Secured)
		if err != nil {
			return err
		}
		return writeNonFPMConf(site, name, rendered)
	case site.IsCustomContainer():
		rendered, err := renderContainerVhost(site, podman.CustomContainerName(site.Name), site.ContainerPort, site.ContainerSSL, site.Secured)
		if err != nil {
			return err
		}
		return writeNonFPMConf(site, name, rendered)
	case site.IsFrankenPHP():
		rendered, err := renderContainerVhost(site, podman.FrankenPHPContainerName(site.Name), podman.FrankenPHPPort, false, site.Secured)
		if err != nil {
			return err
		}
		return writeNonFPMConf(site, name, rendered)
	}
	php := sitePHPVersion(site)
	rendered, err := renderFPMVhost(site, php, site.Secured)
	if err != nil {
		return err
	}
	if err := writeSiteConf(name, keepWaking(site, rendered)); err != nil {
		return err
	}
	syncWorkerSubdomainVhosts(site, php, site.Secured)
	return nil
}
