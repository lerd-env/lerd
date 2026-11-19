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
	proxyTo(w, r, target, func(out *http.Request) {
		out.Host = host
		out.Header.Set("X-Forwarded-Host", host)
		out.Header.Set("X-Forwarded-Proto", forwardedProto(r))
		out.Header.Set("X-Forwarded-Port", serverPort(r))
	})
}

// serveWorker mirrors the vhost's worker location: the request's own Host and
// scheme, since a worker under the app's path is part of the app.
func serveWorker(w http.ResponseWriter, r *http.Request, target *url.URL) {
	proxyTo(w, r, target, func(out *http.Request) {
		out.Host = r.Host
		out.Header.Set("X-Forwarded-Proto", scheme(r))
	})
}

func proxyTo(w http.ResponseWriter, r *http.Request, target *url.URL, headers func(out *http.Request)) {
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Header.Set("X-Forwarded-For", forwardedFor(r))
			pr.Out.Header.Set("X-Real-IP", remoteIP(r))
			headers(pr.Out)
		},
		Transport: insecureBackend,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("lerd web server: proxy %s%s: %v", target.Host, r.URL.Path, err)
			http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		},
	}
	rp.ServeHTTP(w, r)
}

// forwardedHost is nginx's $real_forwarded_host: the host a share tunnel
// forwarded, else the request's own host without its port, as $host is.
func forwardedHost(r *http.Request) string {
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		return h
	}
	return hostOnly(r.Host)
}

func forwardedProto(r *http.Request) string {
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		return p
	}
	return scheme(r)
}

// forwardedFor is $proxy_add_x_forwarded_for: the incoming chain with this
// client appended.
func forwardedFor(r *http.Request) string {
	if prior := r.Header.Get("X-Forwarded-For"); prior != "" {
		return prior + ", " + remoteIP(r)
	}
	return remoteIP(r)
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
