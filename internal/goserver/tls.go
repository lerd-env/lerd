package goserver

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// GetCertificate hands TLS the secured site's own certificate for the name the
// browser asked for. Its subdomains (worktrees, worker subdomains) share it,
// since lerd issues each site's certificate with a wildcard.
func (h *Handler) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if svc, ok := h.serviceFor(hello.ServerName); ok {
		base := filepath.Join(config.CertsDir(), "sites", svc.Domain)
		return h.certs.load(base+".crt", base+".key")
	}
	site, _, ok := h.siteFor(hello.ServerName)
	if !ok || !site.Secured {
		return nil, fmt.Errorf("no secured site serves %q", hello.ServerName)
	}
	base := filepath.Join(config.CertsDir(), "sites", site.PrimaryDomain())
	return h.certs.load(base+".crt", base+".key")
}

// certCache keeps parsed certificates until their file changes, so a reissued
// certificate (a worktree added to the SANs) is picked up without a restart.
type certCache struct {
	mu      sync.Mutex
	entries map[string]cachedCert
}

type cachedCert struct {
	mod  time.Time
	cert *tls.Certificate
}

func (c *certCache) load(certFile, keyFile string) (*tls.Certificate, error) {
	fi, err := os.Stat(certFile)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[certFile]; ok && e.mod.Equal(fi.ModTime()) {
		return e.cert, nil
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	if c.entries == nil {
		c.entries = map[string]cachedCert{}
	}
	c.entries[certFile] = cachedCert{mod: fi.ModTime(), cert: &cert}
	return &cert, nil
}

// httpsURL is the request's own URL over HTTPS, carrying lerd's HTTPS port
// when it has been moved off 443, as the nginx vhost's redirect does.
func httpsURL(r *http.Request) string {
	host := r.Host
	if hst, _, err := net.SplitHostPort(host); err == nil {
		host = hst
	}
	if _, port := config.NginxPorts(); port != 443 {
		host = net.JoinHostPort(host, strconv.Itoa(port))
	}
	return "https://" + host + r.URL.RequestURI()
}
