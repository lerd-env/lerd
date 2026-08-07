package nginx

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// VhostDriftReport says whether the vhost serving a site still matches what
// lerd would write for it. Checked is false for a site whose vhost lerd
// deliberately swapped for something else, where a comparison would only ever
// report the state as damage.
type VhostDriftReport struct {
	Checked bool
	Drifted bool
	Path    string
	Detail  string
}

// VhostDrift renders the vhost lerd would write for a site right now and
// compares it with the file that is serving it. A vhost is written when a site
// is linked, secured, renamed or moved to another PHP version, and nothing
// rewrites it afterwards, so anything that changes what lerd would write leaves
// the file on disk saying something else until someone notices.
func VhostDrift(site config.Site) (VhostDriftReport, error) {
	rendered, ok, err := renderSiteVhost(site)
	if err != nil || !ok {
		return VhostDriftReport{}, err
	}
	path := filepath.Join(config.NginxConfD(), site.PrimaryDomain()+".conf")
	report := VhostDriftReport{Checked: true, Path: path}

	current, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		report.Drifted = true
		report.Detail = "no vhost on disk for " + site.PrimaryDomain()
		return report, nil
	}
	if err != nil {
		return VhostDriftReport{}, err
	}
	if bytes.Equal(current, rendered) {
		return report, nil
	}
	report.Drifted = true
	report.Detail = firstDifference(string(current), string(rendered))
	return report, nil
}

// RegenerateVhost writes the vhost lerd would write for the site now, which is
// what resolves a drifted one. A site whose vhost lerd deliberately swapped is
// left alone, so regenerating never wakes a paused site or drops the waking
// page an idle-suspended host proxy is serving.
func RegenerateVhost(site config.Site) error {
	rendered, ok, err := renderSiteVhost(site)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return writeSiteConf(site.PrimaryDomain()+".conf", rendered)
}

// renderSiteVhost renders the vhost for whichever shape the site is, over HTTPS
// when it is secured, since securing leaves the SSL render at <domain>.conf.
// ok is false when the file serving the site is one lerd swapped in on purpose:
// a paused site's landing page, a suspended host proxy's waking page, or a site
// excluded from serving altogether.
func renderSiteVhost(site config.Site) (rendered []byte, ok bool, err error) {
	if site.Paused || site.Ignored {
		return nil, false, nil
	}
	if site.IsHostProxy() && containsWorker(site.IdleSuspendedWorkers, config.HostProxyWorkerName) {
		return nil, false, nil
	}

	switch {
	case site.IsHostProxy():
		name := "vhost-hostproxy.conf.tmpl"
		if site.Secured {
			name = "vhost-hostproxy-ssl.conf.tmpl"
		}
		rendered, err = renderHostProxyVhost(site, name, site.Secured)
	case site.IsCustomContainer():
		rendered, err = renderContainerVhost(site, podman.CustomContainerName(site.Name), site.ContainerPort, site.ContainerSSL, site.Secured)
	case site.IsFrankenPHP():
		rendered, err = renderContainerVhost(site, podman.FrankenPHPContainerName(site.Name), podman.FrankenPHPPort, false, site.Secured)
	default:
		rendered, err = renderFPMVhost(site, sitePHPVersion(site), site.Secured)
	}
	if err != nil {
		return nil, false, err
	}
	return rendered, true, nil
}

// sitePHPVersion is the version the vhost's fastcgi upstream is named for,
// falling back to the global default the way the reconcile step does.
func sitePHPVersion(site config.Site) string {
	if site.PHPVersion != "" {
		return site.PHPVersion
	}
	if cfg, err := config.LoadGlobal(); err == nil {
		return cfg.PHP.DefaultVersion
	}
	return ""
}

func containsWorker(workers []string, name string) bool {
	for _, w := range workers {
		if w == name {
			return true
		}
	}
	return false
}

// firstDifference names the first line the two versions disagree on, so a
// finding reads as what actually changed rather than as "the file differs".
func firstDifference(current, wanted string) string {
	cur, want := strings.Split(current, "\n"), strings.Split(wanted, "\n")
	for i := 0; i < len(cur) || i < len(want); i++ {
		c, w := lineAt(cur, i), lineAt(want, i)
		if c == w {
			continue
		}
		switch {
		case c == "":
			return fmt.Sprintf("missing %q", w)
		case w == "":
			return fmt.Sprintf("carries %q, which lerd no longer writes", c)
		default:
			return fmt.Sprintf("has %q where lerd would write %q", c, w)
		}
	}
	return "differs from what lerd would write"
}

func lineAt(lines []string, i int) string {
	if i >= len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[i])
}
