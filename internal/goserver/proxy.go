package goserver

import (
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// insecureBackend lets a container that serves its own TLS be proxied to, as
// nginx's proxy_pass https:// does without verifying the backend.
var insecureBackend = &http.Transport{
	Proxy:           nil,
	TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
}

// serveProxied sends the request on with the nginx vhost's headers: the host
// and scheme a share tunnel forwarded, else the request's own, plus client
// address and port. Upgrades (WebSocket, Vite HMR) pass through.
func serveProxied(w http.ResponseWriter, r *http.Request, target *url.URL) {
	host := forwardedHost(r)
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = host
			client, _, _ := net.SplitHostPort(r.RemoteAddr)
			if prior := r.Header.Get("X-Forwarded-For"); prior != "" {
				client = prior + ", " + client
			}
			pr.Out.Header.Set("X-Forwarded-For", client)
			pr.Out.Header.Set("X-Real-IP", remoteIP(r))
			pr.Out.Header.Set("X-Forwarded-Host", host)
			pr.Out.Header.Set("X-Forwarded-Proto", forwardedProto(r))
			pr.Out.Header.Set("X-Forwarded-Port", serverPort(r))
		},
		Transport: insecureBackend,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("lerd web server: proxy %s%s: %v", target.Host, r.URL.Path, err)
			http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		},
	}
	rp.ServeHTTP(w, r)
}

func forwardedHost(r *http.Request) string {
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		return h
	}
	return r.Host
}

func forwardedProto(r *http.Request) string {
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		return p
	}
	return scheme(r)
}

func remoteIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// SiteTarget is where a site served by its own process is proxied to: the
// same host and port the nginx vhost of its kind names. ok is false for a
// PHP-FPM site, which goes over FastCGI instead.
func SiteTarget(site config.Site) (*url.URL, bool) {
	var host string
	var port int
	tls := false
	switch {
	case site.IsHostProxy():
		host, port, tls = podman.HostProxyUpstream(), site.HostPort, site.HostSSL
	case site.IsCustomContainer():
		host, port, tls = podman.CustomContainerName(site.Name), site.ContainerPort, site.ContainerSSL
	case site.IsFrankenPHP():
		host, port = podman.FrankenPHPContainerName(site.Name), podman.FrankenPHPPort
	default:
		return nil, false
	}
	u := &url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(port))}
	if tls {
		u.Scheme = "https"
	}
	return u, true
}
