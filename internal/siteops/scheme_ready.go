package siteops

import (
	"crypto/tls"
	"net/http"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// schemeReadyTimeout bounds the wait for nginx to serve a site's new scheme.
// A reload is quick; this only has to outlast one worker cycle, and a vhost that
// never comes up must report rather than hang the command.
const schemeReadyTimeout = 5 * time.Second

// schemeServed reports whether nginx is already serving the scheme that was just
// configured. A secured site is served once its TLS endpoint answers at all: the
// app's own status says nothing about the vhost, and requiring 200 would wait out
// a site that is merely broken. An unsecured site is served once plain HTTP stops
// redirecting to https, which is what the old secured vhost does until the reload
// takes effect.
func schemeServed(resp *http.Response, err error, secured bool) bool {
	if err != nil || resp == nil {
		return false
	}
	if secured {
		return true
	}
	if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusFound {
		return !strings.HasPrefix(resp.Header.Get("Location"), "https://")
	}
	return true
}

// waitSchemeServed blocks until a request to the site's new scheme shows nginx
// has picked the reload up. Reload is asynchronous, so without this the command
// prints its new URL and returns while the previous vhost is still answering.
func waitSchemeServed(domain string, secured bool, timeout time.Duration) {
	if domain == "" {
		return
	}
	scheme := "http"
	if secured {
		scheme = "https"
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		// The certificate is lerd's own and the redirect is the signal being
		// measured, so neither is followed or verified here.
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	url := scheme + "://" + domain + "/"
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url) //nolint:noctx
		if resp != nil {
			_ = resp.Body.Close()
		}
		if schemeServed(resp, err, secured) {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// waitSchemeServedFn is the indirection point so tests never make real requests.
var waitSchemeServedFn = func(domain string, secured bool) {
	if cfg, err := config.LoadGlobal(); err == nil && !cfg.DNSManaged() && secured {
		// Nothing to probe: HTTPS is unavailable in external-DNS mode.
		return
	}
	waitSchemeServed(domain, secured, schemeReadyTimeout)
}
