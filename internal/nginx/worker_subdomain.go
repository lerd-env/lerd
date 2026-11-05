package nginx

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
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

// servesWorkerSubdomains reports whether the site's own vhost is the FPM one,
// the only shape that serves workers on their own subdomain.
func servesWorkerSubdomains(site config.Site) bool {
	return !site.IsHostProxy() && !site.IsCustomContainer() && !site.IsFrankenPHP()
}

// syncWorkerSubdomainVhosts rewrites the vhosts serving the site's workers on
// their own subdomain and drops any no longer declared. A worktree vhost on the
// same host is removed, since nginx would keep answering from it.
func syncWorkerSubdomainVhosts(site config.Site, phpVersion string, ssl bool) {
	removeWorkerSubdomainVhosts(site.PrimaryDomain())
	vhosts, refused := workerSubdomainVhosts(site, phpVersion, ssl)
	for _, msg := range refused {
		fmt.Fprintln(os.Stderr, "lerd: "+msg)
	}
	for _, label := range slices.Sorted(maps.Keys(vhosts)) {
		_ = RemoveVhost(label + "." + site.PrimaryDomain())
		if err := writeSiteConf(workerVhostConf(site.PrimaryDomain(), label), vhosts[label]); err != nil {
			fmt.Fprintf(os.Stderr, "lerd: worker subdomain %q not served: %v\n", label, err)
		}
	}
}

// workerSubdomainVhosts renders the vhost of each worker served on its own
// subdomain, keyed by label, and names every claim refused. While the site
// waits on a sleeping service its wildcard serves the waking page instead.
func workerSubdomainVhosts(site config.Site, phpVersion string, ssl bool) (map[string][]byte, []string) {
	if siteWaitsOnSleepingService(site.Name) {
		return nil, nil
	}
	fw, ok := config.GetFrameworkForDir(site.Framework, site.Path)
	if !ok {
		return nil, nil
	}
	vhosts := map[string][]byte{}
	owners := map[string]string{}
	var refused []string
	for _, np := range fw.DetectProxies(site.Path) {
		label := np.Proxy.Subdomain
		if label == "" {
			continue
		}
		if first, taken := owners[label]; taken {
			refused = append(refused, fmt.Sprintf("worker %q subdomain not served: %q is already claimed by worker %q", np.Worker, label, first))
			continue
		}
		rendered, err := renderWorkerSubdomainVhost(site, np, phpVersion, ssl)
		if err != nil {
			refused = append(refused, fmt.Sprintf("worker %q subdomain not served: %v", np.Worker, err))
			continue
		}
		owners[label] = np.Worker
		vhosts[label] = rendered
	}
	return vhosts, refused
}

// renderWorkerSubdomainVhost proxies the whole of <label>.<primary> to the
// worker, under the site's certificate, which already carries *.<primary>.
func renderWorkerSubdomainVhost(site config.Site, np config.NamedProxy, phpVersion string, ssl bool) ([]byte, error) {
	label := np.Proxy.Subdomain
	if err := config.ValidateSubdomainLabel(label); err != nil {
		return nil, err
	}
	host := label + "." + site.PrimaryDomain()
	if owner, err := config.FindSiteByDomain(host); err == nil {
		return nil, fmt.Errorf("%s is served by site %q", host, owner.Name)
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
		return nil, err
	}
	tmpl, err := template.New(strings.TrimSuffix(name, ".conf.tmpl")).Parse(string(tmplData))
	if err != nil {
		return nil, err
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
	return renderVhost(tmpl, data)
}

// workerVhostDrift names the first worker vhost on disk that differs from what
// lerd would write: missing, changed, or left over from an undeclared label.
func workerVhostDrift(site config.Site) (path, detail string) {
	primary := site.PrimaryDomain()
	want, _ := workerSubdomainVhosts(site, sitePHPVersion(site), site.Secured)
	for _, label := range slices.Sorted(maps.Keys(want)) {
		path = filepath.Join(config.NginxConfD(), workerVhostConf(primary, label))
		current, err := os.ReadFile(path)
		if err != nil {
			return path, "no worker vhost on disk for " + label + "." + primary
		}
		if !bytes.Equal(current, want[label]) {
			return path, firstDifference(string(current), string(want[label]))
		}
	}
	matches, _ := filepath.Glob(filepath.Join(config.NginxConfD(), workerVhostConf(primary, "*")))
	for _, m := range matches {
		label := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), primary+".worker-"), ".conf")
		if _, ok := want[label]; !ok {
			return m, "carries a worker vhost for " + label + "." + primary + ", which lerd no longer writes"
		}
	}
	return "", ""
}
