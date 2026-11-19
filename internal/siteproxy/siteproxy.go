// Package siteproxy decides which of a site's workers a web server sends
// requests to directly, on a path or a subdomain, and on which port, so every
// web server lerd runs answers the same.
package siteproxy

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
)

// Route is one worker a site's requests reach directly.
type Route struct {
	Worker string
	// Paths are URL prefixes without a trailing slash; "" is the root, which
	// takes every path.
	Paths []string
	// Subdomain is the label the worker is served on, <label>.<primary>.
	Subdomain string
	Port      int
	// OnHost is true for a worker running on the host rather than in the
	// site's PHP container.
	OnHost bool
}

// PathRoutes returns the site's workers served under a URL path.
func PathRoutes(site config.Site) []Route {
	fw, ok := config.GetFrameworkForDir(site.Framework, site.Path)
	if !ok {
		return nil
	}
	var out []Route
	for _, np := range fw.DetectProxies(site.Path) {
		paths := Paths(np.Proxy)
		if len(paths) == 0 {
			continue
		}
		out = append(out, Route{Worker: np.Worker, Paths: paths, Port: Port(site, np), OnHost: np.Proxy.OnHost()})
	}
	return out
}

// Port is the port the worker's server listens on: the one lerd pinned for it,
// the one the site's .env names, or the definition's default.
func Port(site config.Site, np config.NamedProxy) int {
	if np.Proxy.PinnedPort() {
		if p := site.WorkerPorts[np.Worker]; p > 0 {
			return p
		}
	}
	port := np.Proxy.DefaultPort
	if port == 0 {
		port = 8080
	}
	if np.Proxy.PortEnvKey != "" {
		if v := envfile.ReadKey(filepath.Join(site.Path, ".env"), np.Proxy.PortEnvKey); v != "" {
			if p, err := strconv.Atoi(v); err == nil && p > 0 {
				port = p
			}
		}
	}
	return port
}

// Paths normalises the declared paths: a leading slash added, the trailing
// one dropped. "/" becomes "", the root and everything under it, which is what
// a root-mounted worker means.
func Paths(proxy *config.WorkerProxy) []string {
	declared := proxy.Paths
	if len(declared) == 0 {
		declared = []string{proxy.Path}
	}
	var paths []string
	for _, raw := range declared {
		if raw == "" {
			continue
		}
		p := strings.TrimSuffix(raw, "/")
		if p != "" && !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		paths = append(paths, p)
	}
	return paths
}

// Subdomains returns the site's workers served on a subdomain of their own,
// keyed by label, and names every claim refused: a label claimed by an earlier
// worker, one that is not a valid host label, or a host a registered site owns.
func Subdomains(site config.Site) (map[string]Route, []string) {
	if !site.ServesWorkerSubdomains() {
		return nil, nil
	}
	fw, ok := config.GetFrameworkForDir(site.Framework, site.Path)
	if !ok {
		return nil, nil
	}
	routes := map[string]Route{}
	var refused []string
	for _, np := range fw.DetectProxies(site.Path) {
		label := np.Proxy.Subdomain
		if label == "" {
			continue
		}
		if first, taken := routes[label]; taken {
			refused = append(refused, fmt.Sprintf("worker %q subdomain not served: %q is already claimed by worker %q", np.Worker, label, first.Worker))
			continue
		}
		if err := config.ValidateSubdomainLabel(label); err != nil {
			refused = append(refused, fmt.Sprintf("worker %q subdomain not served: %v", np.Worker, err))
			continue
		}
		host := label + "." + site.PrimaryDomain()
		if owner, err := config.FindSiteByDomain(host); err == nil {
			refused = append(refused, fmt.Sprintf("worker %q subdomain not served: %s is served by site %q", np.Worker, host, owner.Name))
			continue
		}
		routes[label] = Route{Worker: np.Worker, Subdomain: label, Port: Port(site, np), OnHost: np.Proxy.OnHost()}
	}
	return routes, refused
}
