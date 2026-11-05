package nginx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/geodro/lerd/internal/config"
)

// workerVhostConf names a worker subdomain's vhost after its site rather than
// its host, so the worktree sweeps over *.<primary>.conf never take it.
func workerVhostConf(primary, label string) string {
	return primary + ".worker-" + label + ".conf"
}

func removeWorkerSubdomainVhosts(primary string) {
	matches, _ := filepath.Glob(filepath.Join(config.NginxConfD(), workerVhostConf(primary, "*")))
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

// syncWorkerSubdomainVhosts rewrites the vhosts serving the site's workers on
// their own subdomain and drops any no longer declared. While the site waits on
// a sleeping service its wildcard serves the waking page for them instead.
func syncWorkerSubdomainVhosts(site config.Site, phpVersion string, ssl bool) {
	removeWorkerSubdomainVhosts(site.PrimaryDomain())
	if siteWaitsOnSleepingService(site.Name) {
		return
	}
	fw, ok := config.GetFrameworkForDir(site.Framework, site.Path)
	if !ok {
		return
	}
	for _, np := range fw.DetectProxies(site.Path) {
		if np.Proxy.Subdomain == "" {
			continue
		}
		if err := writeWorkerSubdomainVhost(site, np, phpVersion, ssl); err != nil {
			fmt.Fprintf(os.Stderr, "lerd: worker %q subdomain not served: %v\n", np.Worker, err)
		}
	}
}

// writeWorkerSubdomainVhost proxies the whole of <label>.<primary> to the
// worker, under the site's certificate, which already carries *.<primary>.
func writeWorkerSubdomainVhost(site config.Site, np config.NamedProxy, phpVersion string, ssl bool) error {
	label := np.Proxy.Subdomain
	if err := config.ValidateSubdomainLabel(label); err != nil {
		return err
	}
	host := label + "." + site.PrimaryDomain()
	if owner, err := config.FindSiteByDomain(host); err == nil {
		return fmt.Errorf("%s is served by site %q", host, owner.Name)
	}
	upstream, _ := fpmUpstream(&site, phpVersion)
	if np.Proxy.OnHost() {
		upstream = hostProxyUpstream()
	}
	name := "vhost-custom.conf.tmpl"
	if ssl {
		name = "vhost-custom-ssl.conf.tmpl"
	}
	tmplData, err := GetTemplate(name)
	if err != nil {
		return err
	}
	tmpl, err := template.New(strings.TrimSuffix(name, ".conf.tmpl")).Parse(string(tmplData))
	if err != nil {
		return err
	}
	data := VhostData{
		Domain:          host,
		ServerNames:     host,
		CustomContainer: upstream,
		CustomPort:      proxyPort(site, np),
		RequestTimeout:  resolveRequestTimeout(site.Path, phpVersion),
	}
	if ssl {
		data.CertDomain = site.PrimaryDomain()
	}
	rendered, err := renderVhost(tmpl, data)
	if err != nil {
		return err
	}
	return writeSiteConf(workerVhostConf(site.PrimaryDomain(), label), rendered)
}
