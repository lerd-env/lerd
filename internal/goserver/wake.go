package goserver

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"slices"

	"github.com/geodro/lerd/internal/config"
)

// WakeHoldPath is lerd-ui's endpoint a sleeping site's requests are held at.
const WakeHoldPath = "/_lerd/wake"

// wakeHoldFailed is the status lerd-ui answers when the hold itself failed,
// the one code turned into the waking page so the app's own errors pass.
const wakeHoldFailed = 599

var errHoldFailed = errors.New("wake hold failed")

// Waking reports whether the site's requests wait for it to wake: a host-proxy
// site lerd idle-suspended, or one whose service lerd has put to sleep.
func Waking(site config.Site) bool {
	if site.IsHostProxy() && slices.Contains(site.IdleSuspendedWorkers, config.HostProxyWorkerName) {
		return true
	}
	return config.SiteWaitsOnSleepingService(site.Name)
}

// serveWakeHold hands the request, method and body included, to lerd-ui's
// hold, which wakes the site and answers with the app's own response, as the
// waking vhost does. A hold that fails or cannot be reached gets the waking page.
func (h *Handler) serveWakeHold(w http.ResponseWriter, r *http.Request) {
	skipAccessFeed(w)
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(h.WakeHold)
			pr.Out.URL.Path, pr.Out.URL.RawPath, pr.Out.URL.RawQuery = WakeHoldPath, "", ""
			pr.Out.Header.Set("X-Lerd-Wake-Host", hostOnly(r.Host))
			pr.Out.Header.Set("X-Lerd-Wake-Uri", r.URL.RequestURI())
			pr.Out.Header.Set("X-Lerd-Wake-Scheme", scheme(r))
		},
		Transport:     h.wakeTransport(),
		FlushInterval: -1,
		ModifyResponse: func(resp *http.Response) error {
			switch resp.StatusCode {
			case http.StatusBadGateway, http.StatusGatewayTimeout, wakeHoldFailed:
				resp.Body.Close()
				return errHoldFailed
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) { serveWakingPage(w) },
	}
	rp.ServeHTTP(w, r)
}

func (h *Handler) wakeTransport() http.RoundTripper {
	if h.WakeTransport != nil {
		return h.WakeTransport
	}
	return http.DefaultTransport
}

func serveWakingPage(w http.ResponseWriter) {
	page, err := os.ReadFile(filepath.Join(config.PausedDir(), "waking.html"))
	if err != nil {
		http.Error(w, "503 Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page)
}

// skipAccessFeed keeps a held request off the access feed: it is not the
// app's, and the hold reports the site's activity itself.
func skipAccessFeed(w http.ResponseWriter) {
	if sw, ok := w.(*statusWriter); ok {
		sw.skip = true
	}
}
