package sitedoctor

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// httpCheckTimeout bounds the request. A root that needs longer than this is a
// finding of its own, and the doctor must not hang on a wedged pool.
const httpCheckTimeout = 10 * time.Second

// httpProbe is the seam tests replace, so the check can be exercised without a
// site actually listening on the machine running the tests.
var httpProbe = func(url string) (int, error) {
	client := &http.Client{
		Timeout: httpCheckTimeout,
		// The site's own certificate is the cert checks' subject, not this one's:
		// a stale or untrusted cert should read as "answered", so that finding
		// stays where it belongs instead of surfacing here as a dead site.
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		// A redirect is an answer. Following it would report the destination's
		// status instead, and an http site redirecting to https is the norm.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// nginxUp is hooked for the same reason.
var nginxUp = func() bool { return podman.ContainerRunningQuiet("lerd-nginx") }

// checkHTTPResponse asks the site for its root and reports what came back.
// Every other check here reads configuration, and configuration can be perfect
// while the site serves nothing: a PHP pin the installed dependencies cannot
// satisfy, a pool that never came back, a fatal in the app itself. This is the
// only check that finds out whether the site answers at all.
func checkHTTPResponse(path string) (Check, bool) {
	site, err := config.FindSiteByPath(path)
	if err != nil || site == nil {
		return Check{}, false
	}
	url := siteRootURL(*site)
	if url == "" {
		return Check{}, false
	}
	// With nginx down every site fails identically and none of it is the site's
	// doing, so say what is actually wrong instead of blaming the app.
	if !nginxUp() {
		return Check{
			Name:   "http response",
			Status: StatusWarn,
			Detail: "skipped — lerd-nginx is not running (start it with: lerd start)",
		}, true
	}

	code, err := httpProbe(url)
	if err != nil {
		return Check{
			Name:   "http response",
			Status: StatusFail,
			Detail: fmt.Sprintf("%s did not answer: %v", url, err),
			Fix:    "",
		}, true
	}
	c := Check{Name: "http response"}
	switch {
	case code >= 500:
		c.Status = StatusFail
		c.Detail = fmt.Sprintf("%s answered %d — the app is erroring, read it with: lerd logs", url, code)
	default:
		// A 4xx is not a failure. A framework skeleton with no routes answers 404
		// on / by design, and nothing here can tell that apart from a real one.
		c.Status = StatusOK
		c.Detail = fmt.Sprintf("%s answered %d", url, code)
	}
	return c, true
}

// siteRootURL is the address the site is served on, carrying a non-default
// nginx port so a host that moved 80/443 is probed where it actually listens.
func siteRootURL(site config.Site) string {
	domain := site.PrimaryDomain()
	if domain == "" {
		return ""
	}
	httpPort, httpsPort := config.NginxPorts()
	scheme, bind, standard := "http", httpPort, 80
	if site.Secured {
		scheme, bind, standard = "https", httpsPort, 443
	}
	host := domain
	if bind != standard {
		host = net.JoinHostPort(domain, strconv.Itoa(bind))
	}
	return scheme + "://" + host
}
