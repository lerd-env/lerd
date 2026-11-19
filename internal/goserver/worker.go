package goserver

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteproxy"
)

// workerSubdomain is the worker served on <label>.<domain> when the request
// is for such a host, the whole host going to it as nginx's own vhost does.
func (h *Handler) workerSubdomain(site config.Site, domain, hostport string) (*url.URL, bool) {
	if h.WorkerHost == nil {
		return nil, false
	}
	label, ok := strings.CutSuffix(strings.ToLower(hostOnly(hostport)), "."+domain)
	if !ok || strings.Contains(label, ".") {
		return nil, false
	}
	routes, _ := siteproxy.Subdomains(site)
	r, ok := routes[label]
	if !ok {
		return nil, false
	}
	return h.workerURL(site, r), true
}

// workerPath is the worker whose path the request falls under, anchored as
// the nginx location ^<path>(/|$) is: /app and /app/x, never /appfoo.
func (h *Handler) workerPath(site config.Site, urlPath string) (*url.URL, bool) {
	if h.WorkerHost == nil {
		return nil, false
	}
	for _, r := range siteproxy.PathRoutes(site) {
		for _, p := range r.Paths {
			if p == "" || urlPath == p || strings.HasPrefix(urlPath, p+"/") {
				return h.workerURL(site, r), true
			}
		}
	}
	return nil, false
}

func (h *Handler) workerURL(site config.Site, r siteproxy.Route) *url.URL {
	return &url.URL{Scheme: "http", Host: net.JoinHostPort(h.WorkerHost(site, r.OnHost), strconv.Itoa(r.Port))}
}
