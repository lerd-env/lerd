package goserver

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
)

// ServiceRoute is a service served on its own domain, such as an object store
// whose presigned URLs the browser has to reach under the name they were
// signed for.
type ServiceRoute struct {
	Domain string
	// Upstream is the service container's host:port.
	Upstream string
	// CORS answers browser preflights for pages on lerd's own domains.
	CORS bool
	// TLD is lerd's domain suffix, which bounds the origins CORS reflects.
	TLD string
}

var tldLabel = regexp.MustCompile(`^[a-z0-9]+(\.[a-z0-9]+)*$`)

// corsOrigin is the Origin to reflect, or "" for one lerd does not serve,
// matching the nginx vhost's ^https?://[^/]+\.<tld>(:port)?$.
func (s ServiceRoute) corsOrigin(origin string) string {
	tld := s.TLD
	if !tldLabel.MatchString(tld) {
		tld = "test"
	}
	re := regexp.MustCompile(`^https?://[^/]+\.` + strings.ReplaceAll(tld, ".", `\.`) + `(:[0-9]+)?$`)
	if re.MatchString(origin) {
		return origin
	}
	return ""
}

func (h *Handler) serviceFor(hostport string) (ServiceRoute, bool) {
	if h.Services == nil {
		return ServiceRoute{}, false
	}
	host := strings.ToLower(hostport)
	if hst, _, err := net.SplitHostPort(host); err == nil {
		host = hst
	}
	for _, s := range h.Services() {
		if s.Domain == host {
			return s, true
		}
	}
	return ServiceRoute{}, false
}

var corsHeaders = []string{
	"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers",
	"Access-Control-Expose-Headers", "Access-Control-Max-Age",
}

// serveService mirrors the service-domain vhost: HTTPS only, the Host kept for
// request signing, nothing buffered, and with CORS on the upstream's own CORS
// headers replaced by lerd's.
func serveService(w http.ResponseWriter, r *http.Request, s ServiceRoute) {
	if r.TLS == nil {
		http.Redirect(w, r, "https://"+hostOnly(r.Host)+r.URL.RequestURI(), http.StatusMovedPermanently)
		return
	}
	allowed := ""
	if s.CORS {
		allowed = s.corsOrigin(r.Header.Get("Origin"))
		if r.Method == http.MethodOptions {
			hd := w.Header()
			if allowed != "" {
				hd.Set("Access-Control-Allow-Origin", allowed)
			}
			hd.Set("Access-Control-Allow-Methods", "GET, PUT, POST, DELETE, HEAD, OPTIONS")
			// Reflected rather than "*", which Safari does not accept here.
			if req := r.Header.Get("Access-Control-Request-Headers"); req != "" {
				hd.Set("Access-Control-Allow-Headers", req)
			}
			hd.Set("Access-Control-Max-Age", "86400")
			hd.Set("Vary", "Origin")
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(&url.URL{Scheme: "http", Host: s.Upstream})
			pr.Out.Host = r.Host
			pr.Out.Header.Set("X-Forwarded-For", forwardedFor(r))
			pr.Out.Header.Set("X-Real-IP", remoteIP(r))
			pr.Out.Header.Set("X-Forwarded-Proto", "https")
		},
		FlushInterval: -1,
		ModifyResponse: func(resp *http.Response) error {
			if !s.CORS {
				return nil
			}
			for _, k := range corsHeaders {
				resp.Header.Del(k)
			}
			if allowed != "" {
				resp.Header.Set("Access-Control-Allow-Origin", allowed)
			}
			resp.Header.Set("Access-Control-Expose-Headers", "ETag")
			resp.Header.Set("Vary", "Origin")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("lerd web server: service %s: %v", s.Domain, err)
			http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		},
	}
	rp.ServeHTTP(w, r)
}

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}
